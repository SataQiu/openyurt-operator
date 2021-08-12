#!/usr/bin/env sh

# Copyright 2021 The OpenYurt Authors.
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

IS_CONVERT=${1:-"false"}
KUBERNETES_SERVICE_HOST=${2:-"10.96.0.1"}

echo "convert: $IS_CONVERT"

HostWorkDir="/var/tmp"

for id in $(${HostWorkDir}/crictl ps -q)
do
  # skip system pods of edge (yurt*)
  name=$(${HostWorkDir}/crictl inspect $id | ${HostWorkDir}/jq -r .status.metadata.name)
  if case $name in "yurt"*) true;; *) false;; esac; then
    echo "skip system pod $id"
    continue
  fi
  # check if .info.config.envs available
  envs=$(${HostWorkDir}/crictl inspect $id | ${HostWorkDir}/jq .info.config.envs)
  if [ "$envs" = "" ] || [ "$envs" = "null" ]; then
    # we can not check whether the Pod env is right, so just stop it now
    sandboxID=$(${HostWorkDir}/crictl ps -o json | ${HostWorkDir}/jq .containers | ${HostWorkDir}/jq -c ".[] | select(.id == \"$id\")" | ${HostWorkDir}/jq -r .podSandboxId)
    if [ "$sandboxID" != "" ]; then
      echo "stop $id without env check"
      ${HostWorkDir}/crictl stopp $sandboxID
    fi
    continue
  fi
  addr=$(${HostWorkDir}/crictl inspect $id | ${HostWorkDir}/jq .info.config.envs | ${HostWorkDir}/jq -c '.[] | select(.key == "KUBERNETES_SERVICE_HOST")' | ${HostWorkDir}/jq -r .value)
  if [ "$addr" = "" ]; then
    continue
  fi
  if [ "$IS_CONVERT" = "true" ]; then
    if [ "$addr" = "$KUBERNETES_SERVICE_HOST" ]; then
      echo "stop $id, addr $addr, kubernetes service host $KUBERNETES_SERVICE_HOST"
      ${HostWorkDir}/crictl stopp $(${HostWorkDir}/crictl inspect $id | ${HostWorkDir}/jq -r .info.sandboxID)
    fi
  else
    if [ "$addr" != "$KUBERNETES_SERVICE_HOST" ]; then
      echo "stop $id, addr $addr, kubernetes service host $KUBERNETES_SERVICE_HOST"
      ${HostWorkDir}/crictl stopp $(${HostWorkDir}/crictl inspect $id | ${HostWorkDir}/jq -r .info.sandboxID)
    fi
  fi
done
