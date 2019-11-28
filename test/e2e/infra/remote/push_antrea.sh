#!/usr/bin/env bash
# Copyright 2019 Antrea Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

: "${SSH_CONFIG:=$HOME/.ssh/config}"
SAVED_IMG=/tmp/antrea-ubuntu.tar
IMG_NAME=antrea/antrea-ubuntu:latest
HOSTS=( "$@" )
HOSTS_NUM=${#HOSTS[@]}
THIS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

pushd "$THIS_DIR" || exit

ANTREA_YML=$THIS_DIR/../../../../build/yamls/antrea.yml

if [ ! -f "$SSH_CONFIG" ]; then
  echo "SSH config file $SSH_CONFIG does not exist"
  echo "Please create the $SSH_CONFIG file or run with SSH_CONFIG=<ssh-config path> $0"
  exit 1
fi

docker inspect $IMG_NAME >/dev/null
if [ $? -ne 0 ]; then
  echo "Docker image $IMG_NAME was not found"
  exit 1
fi

echo "Saving $IMG_NAME image to $SAVED_IMG"
docker save -o $SAVED_IMG $IMG_NAME

function waitForNodes() {
  pids=("$@")
  for pid in "${pids[@]}"; do
    if ! wait "$pid"; then
      echo "Command failed for one or more node"
      wait # wait for all remaining processes
      exit 2
    fi
  done
}

echo "Copying $IMG_NAME image to every node..."
# Loop over all hosts and copy image to each one
for ((i = 0; i < HOSTS_NUM; i++)); do
  host="${HOSTS[i]}"
  echo "$host"
  scp -F "$SSH_CONFIG" "$SAVED_IMG" "$host":"$SAVED_IMG" &
  pids[$i]=$!
done

# Wait for all child processes to complete
waitForNodes "${pids[@]}"
echo "Done!"

echo "Loading $IMG_NAME image in every node..."
# Loop over all hosts and copy image to each one
for ((i = 0; i < HOSTS_NUM; i++)); do
  host="${HOSTS[i]}"
  ssh -F "$SSH_CONFIG" "$host" docker load -i /tmp/antrea-ubuntu.tar &
  pids[$i]=$!
done
# Wait for all child processes to complete
waitForNodes "${pids[@]}"
echo "Done!"

echo "Copying Antrea deployment YAML to every node..."
# Loop over all hosts and copy image to each one
for ((i = 0; i < HOSTS_NUM; i++)); do
  host="${HOSTS[i]}"
  scp -F "$SSH_CONFIG" "$ANTREA_YML" "$host":~/ &
  pids[$i]=$!
done
# Wait for all child processes to complete
waitForNodes "${pids[@]}"
echo "Done!"

# To ensure that the most recent version of Antrea (that we just pushed) will be used.
echo "Restarting Antrea DaemonSet"
ssh -F "$SSH_CONFIG" "$1" kubectl -n kube-system delete all -l app=antrea
ssh -F "$SSH_CONFIG" "$1" kubectl apply -f antrea.yml
echo "Done!"
