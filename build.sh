#!/usr/bin/env bash

rm -rf build

mkdir build

go build -o build/vhugo cli/cli.go

mv build/vhugo vhugo

rm -rf build

scp vhugo goservice:/tmp

ssh goservice sudo /tmp/vhugo -action deploy
