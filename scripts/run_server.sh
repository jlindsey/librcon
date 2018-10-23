#!/bin/bash

SERVER_URL="https://launcher.mojang.com/v1/objects/fe123682e9cb30031eae351764f653500b7396c9/server.jar"

pushd ./test
if ! [[ -e "server.jar" ]]; then
    curl -L -o server.jar "$SERVER_URL"
fi
echo "eula=true" > eula.txt
cp server.properties.local server.properties
java -server -jar server.jar nogui
popd