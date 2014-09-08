#!/bin/sh

SPATH=$(cd "$(dirname "$0")"; pwd)

SVN_PATH="/macken/build/GoCheck"
BUILD_PATH="/macken/build/buildgocheck"
DST_PATH="/macken/gocheck/"

cd $SVN_PATH
git pull
rsync -az --delete --exclude=".git" $SVN_PATH/* $BUILD_PATH/

cp -f $BUILD_PATH/src/*.go $DST_PATH
cp -f $BUILD_PATH/doc/*.sh $DST_PATH
cp -f $BUILD_PATH/doc/pubgocheck.sh /macken/

GOROOT="/usr/local/go"
GOPATH="/macken/go/"

cd $DST_PATH
rm -rf checker scanner server
$GOROOT/bin/go build checker.go
$GOROOT/bin/go build scanner.go
$GOROOT/bin/go build server.go

