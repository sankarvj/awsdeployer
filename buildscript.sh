#!/bin/bash

echo "cleaned old deployment."
rm -rf deployment.zip
echo "building main..."
GOOS=linux go build -o main
zip -r deployment.zip main environment codedeploy
echo "Build successfull. Output stored at deployment.zip"