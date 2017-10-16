## Kafka Queue Worker for OpenFaaS - Kafka Streaming

This is work in progress - find out more here: https://github.com/openfaas/faas/issues/234.

Some design considerations:

* Follow current code/flow for nats queue worker; add code in gateway and add kafka_queue_worker repository.

* Add docker-compose.kafka.yml to bring up zookeeper & kafka nodes in swarm together with gateway and kafka-queue-worker.

* To avoid tool-chain change, use Shopify sarama package instead of confluent-kafka-go package. While the latter quarantees the latest features, it introduces dependencies on C/C++ librdkafka library. We may not be able to build static linked binaries and have to use [alpine images with librdkafka built in](http://github.com/yglcode/alpine-kafka-go).

How to test:

* Clone this repo, cd queue-work/, run "./build.sh"; then push it to your registry.

* Check out my "kafka-queue-worker" branch of openfaas/faas, cd faas/gateway/, run "./build.sh"; then push it to your registry.

* setup env:
  * REGISTRY_SLASH="your_registry/"
  * COLON_ETAG=":latest-dev"
  * switch to swarm master docker env: eval $(docker-machine env master)
  
* deploy the stack:
  * cd faas/
  * docker stack deploy -c docker-compose.kafka.yml kk

* check the messaging:
  * docker service logs -f kk_kafka-queue-worker


  