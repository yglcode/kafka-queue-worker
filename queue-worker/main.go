package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"time"
	"strings"
	"math"

	"net/http"

	"github.com/openfaas/faas/gateway/queue"
	"github.com/Shopify/sarama"
	"github.com/bsm/sarama-cluster"
)

// Sarama currently cannot support latest kafka protocol version 0_11_
var (
	SARAMA_KAFKA_PROTO_VER = sarama.V0_10_2_0
)

// AsyncReport is the report from a function executed on a queue worker.
type AsyncReport struct {
	FunctionName string  `json:"name"`
	StatusCode   int     `json:"statusCode"`
	TimeTaken    float64 `json:"timeTaken"`
}

func makeClient() http.Client {
	proxyClient := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   3 * time.Second,
				KeepAlive: 0,
			}).DialContext,
			MaxIdleConns:          1,
			DisableKeepAlives:     true,
			IdleConnTimeout:       120 * time.Millisecond,
			ExpectContinueTimeout: 1500 * time.Millisecond,
		},
	}
	return proxyClient
}

func main() {
	log.SetFlags(0)

	kafkaBrokers := []string{"kafka1:19092"}
	group := "faas-kafka-queue-workers"
	topics := []string{"faas-request"}
	gatewayAddress := "gateway"
	functionSuffix := ""

	if val, exists := os.LookupEnv("faas_kafka_brokers"); exists {
		kafkaBrokers = strings.Split(val,",")
		for i:=0;i<len(kafkaBrokers); {
			if len(kafkaBrokers[i])==0 {
				kafkaBrokers = append(kafkaBrokers[:i],kafkaBrokers[i+1:]...)
			} else {
				i++
			}
		}
	}

	if val, exists := os.LookupEnv("faas_kafka_group"); exists {
		group = val
	}

	if val, exists := os.LookupEnv("faas_queue_topics"); exists {
		topics = strings.Split(val,",")
		for i:=0;i<len(topics); {
			if len(topics[i])==0 {
				topics = append(topics[:i],topics[i+1:]...)
			} else {
				i++
			}
		}
	}

	if val, exists := os.LookupEnv("faas_gateway_address"); exists {
		gatewayAddress = val
	}

	if val, exists := os.LookupEnv("faas_function_suffix"); exists {
		functionSuffix = val
	}

	// Kafka-queue-worker and zookeeper & kafka brokers may start at same time
	// Wait for that kafka is up and topics are provisioned
	waitforBrokersTopics(kafkaBrokers,topics)

	//setup consumer
	cConfig := cluster.NewConfig()
	cConfig.Version = SARAMA_KAFKA_PROTO_VER
	cConfig.Consumer.Return.Errors = true
	cConfig.Consumer.Offsets.Initial = sarama.OffsetNewest  //OffsetOldest
	cConfig.Group.Return.Notifications = true
	cConfig.Group.Session.Timeout = 6 * time.Second
	cConfig.Group.Heartbeat.Interval = 2 * time.Second

	consumer, err := cluster.NewConsumer(kafkaBrokers, group, topics, cConfig)
	if err != nil {
		log.Fatalln("Fail to create Kafka consumer: ",err)
	}
	defer consumer.Close()

	fmt.Printf("Created Consumer %v\n", consumer)

	client := makeClient()

	mcb := func(req *queue.Request) {

		started := time.Now()

		fmt.Printf("Request for %s.\n", req.Function)
		urlFunction := fmt.Sprintf("http://%s%s:8080/", req.Function, functionSuffix)

		request, err := http.NewRequest("POST", urlFunction, bytes.NewReader(req.Body))
		defer request.Body.Close()

		res, err := client.Do(request)
		var status int
		var functionResult []byte

		if err != nil {
			status = http.StatusServiceUnavailable

			log.Println(err)
			timeTaken := time.Since(started).Seconds()

			if req.CallbackURL != nil {
				log.Printf("Callback to: %s\n", req.CallbackURL.String())
				postResult(&client, *req, functionResult, status)
			}

			postReport(&client, req.Function, status, timeTaken, gatewayAddress)
			return
		}

		if res.Body != nil {
			defer res.Body.Close()

			resData, err := ioutil.ReadAll(res.Body)
			functionResult = resData

			if err != nil {
				log.Println(err)
			}
			fmt.Println(string(functionResult))
		}
		timeTaken := time.Since(started).Seconds()
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(res.Status)

		if req.CallbackURL != nil {
			log.Printf("Callback to: %s\n", req.CallbackURL.String())
			postResult(&client, *req, functionResult, res.StatusCode)
		}

		postReport(&client, req.Function, res.StatusCode, timeTaken, gatewayAddress)
	}

	// Wait for a SIGINT (perhaps triggered by user with CTRL-C)
	// Run cleanup when signal is received
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	num:=0
MAINLOOP:
	for {
		select {
		case _ = <- signalChan:
			fmt.Printf("\nReceived an interrupt, exit...\n\n")
			break MAINLOOP
		case msg, ok := <-consumer.Messages():
			if ok {
				num = (num+1)%math.MaxInt32
				fmt.Printf("[#%d] Received on [%v,%v]: '%s'\n", num, msg.Topic, msg.Partition, string(msg.Value))
				req := queue.Request{}
				json.Unmarshal(msg.Value, &req)
				mcb(&req)
				consumer.MarkOffset(msg, "") // mark message as processed
			}
		case err = <- consumer.Errors():
			fmt.Println("consumer error: ",err)
		case ntf := <- consumer.Notifications():
			fmt.Println("Rebalanced: %+v\n", ntf)
		}
	}
}

func postResult(client *http.Client, req queue.Request, result []byte, statusCode int) {
	var reader io.Reader

	if result != nil {
		reader = bytes.NewReader(result)
	}

	request, err := http.NewRequest("POST", req.CallbackURL.String(), reader)
	res, err := client.Do(request)

	if err != nil {
		log.Printf("Error posting result to URL %s %s\n", req.CallbackURL.String(), err.Error())
		return
	}

	if request.Body != nil {
		defer request.Body.Close()
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	log.Printf("Posting result - %d\n", res.StatusCode)
}

func postReport(client *http.Client, function string, statusCode int, timeTaken float64, gatewayAddress string) {
	req := AsyncReport{
		FunctionName: function,
		StatusCode:   statusCode,
		TimeTaken:    timeTaken,
	}

	reqBytes, _ := json.Marshal(req)
	request, err := http.NewRequest("POST", "http://"+gatewayAddress+":8080/system/async-report", bytes.NewReader(reqBytes))
	defer request.Body.Close()

	res, err := client.Do(request)

	if err != nil {
		log.Println("Error posting report", err)
		return
	}
	if res.Body != nil {
		defer res.Body.Close()
	}
	log.Printf("Posting report - %d\n", res.StatusCode)

}

// OpenFaas gateway and zookeeper & kafka brokers may start at same time
// wait for that kafka is up and topics are provisioned
func waitforBrokersTopics(brokers []string, topics []string) {
	var client sarama.Client 
	var err error
	for {
		client,err = sarama.NewClient(brokers,nil)
		if client!=nil && err==nil { break }
		if client!=nil { client.Close() }
		fmt.Println("Wait for kafka brokers coming up...")
		time.Sleep(2*time.Second)
	}
	fmt.Println("Kafka brokers up")
	count := len(topics)
LOOP_TOPIC:
	for {
		tops,err := client.Topics()
		if tops!=nil && err==nil {
			for _,t1 := range tops {
				for _,t2 := range topics {
					if t1==t2 {
						fmt.Println("Topic ",t2," is ready")
						count--
						if count==0 { // All expected topics ready
							break LOOP_TOPIC
						} else {
							break
						}
					}
				}
			}
		}
		fmt.Println("Wait for topics:",topics)
		client.RefreshMetadata(topics...)
		time.Sleep(2*time.Second)
	}
	client.Close()
}
