#!/bin/bash

WEBSERVER_PORT=9000 \
KEYFILE="/etc/letsencrypt/live/lccny6.drewles.com/privkey.pem" \
CERTFILE="/etc/letsencrypt/live/lccny6.drewles.com/fullchain.pem" \
go run main.go