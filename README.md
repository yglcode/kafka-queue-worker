## Kafka Queue Worker for OpenFaaS - Kafka Streaming

This is work in progress - find out more here: https://github.com/openfaas/faas/issues/234.

Some design considerations:

* Follow current code/flow for nats queue worker; add code in gateway and add kafka_queue_worker repository.

* Add docker-compose.kafka-queue.yml to bring up zookeeper & kafka nodes in swarm together with gateway and kafka-queue-worker.

* To avoid tool-chain change, use Shopify sarama package instead of confluent-kafka-go package. While the latter quarantees the latest features, it introduces dependencies on C/C++ librdkafka library. We may not be able to build static linked binaries and have to use [alpine images with librdkafka built in](http://github.com/yglcode/alpine-kafka-go).

How to test:

* Clone this repo, cd queue-worker/, run "./build.sh"; then push kafka-queue-worker:latest-dev to your registry.

* Check out ["kafka_queue_worker" branch](http://github.com/yglcode/faas/tree/kafka_queue_worker), cd faas/gateway/, run "./build.sh"; then push gateway:latest-dev to your registry.

* Setup env:
  * switch to swarm master docker env: eval $(docker-machine env master)
  
* Deploy the stacks:
         zookeeper+kafka test cluster and gateway+kafka-queue-worker+functions
  * cd faas/
  * ./deploy_kafka_queue.sh

* Test call async functions:
  for i in {1..10}; do curl http://gateway:8080/async-function/func_echoit --data "hi"; done 

* Check the messaging:
  * docker service logs -f func_kafka-queue-worker
