#!/bin/bash

for i in {1..5}
do
  result=$(curl -s http://localhost:8080/healthcheck)
  if [[ -z "${result// }" ]]; then
    exit 1
  fi
  if [[ $result != *"ok"* ]]; then
    exit 1
  fi
done
echo "all good"
exit 0
