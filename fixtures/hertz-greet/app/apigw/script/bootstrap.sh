#!/bin/bash
CURDIR=$(cd $(dirname $0); pwd)
BinaryName=greet
echo "$CURDIR/bin/${BinaryName}"
exec $CURDIR/bin/${BinaryName}