#!/bin/bash
sudo docker build --build-arg MGPROXY_VERSION=v1.0 -t massgrid/10.0-ubuntu16.04-proxy:v1.0 https://raw.githubusercontent.com/wanyvic/mgproxy/master/Dockerfile
sudo docker push massgrid/10.0-ubuntu16.04-proxy:v1.0