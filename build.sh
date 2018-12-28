#!/usr/bin/env bash

rm -rf build

mkdir build

go build -o build/vhugo cli/cli.go

cd web

zip ../build/web.zip -q -r *

cd ..

cat build/web.zip >> build/vhugo

zip -q -A build/vhugo

mv build/vhugo vhugo

rm -rf build

./vhugo -ip x.x.x.x
