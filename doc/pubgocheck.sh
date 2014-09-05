#!/bin/sh

SPATH=$(cd "$(dirname "$0")"; pwd)

SVN_PATH="/macken/build/GoCheck"
BUILD_PATH="/macken/build/buildgocheck"

cd $SVN_PATH
git pull
rsync -az --delete --exclude=".git" $SVN_PATH/* $BUILD_PATH/

cp -f $BUILD_PATH/src/*.go /macken/gocheck/
cp -f $BUILD_PATH/doc/pubgocheck.sh /macken
