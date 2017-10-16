#!/bin/sh

export eTAG="latest-dev"
echo $1
if [ $1 ] ; then
  eTAG=$1
fi

echo Building functions/kafka-queue-worker:$eTAG

docker build --build-arg http_proxy=$http_proxy -t functions/kafka-queue-worker:$eTAG .

