#!/bin/bash

pushd ./test
for file in "$(cat .gitignore)"; do
    rm -rf $file
done
popd