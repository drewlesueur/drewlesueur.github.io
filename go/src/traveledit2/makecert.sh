#!/bin/bash

sudo apt-get update
sudo apt-get install software-properties-common
sudo add-apt-repository universe
sudo apt-get update

sudo apt-get install certbot

sudo certbot certonly --standalone
