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

# This script is used in yurt-agent to run revert locally
# Please DO NOT run this script directly.

set -o errexit
set -o nounset
set -o pipefail

HostWorkDir="/var/tmp"
HostWorkMountDir="/tmp"

function prepareResources {
  cp -a /var/run/secrets/kubernetes.io/serviceaccount ${HostWorkMountDir}/serviceaccount
}

function cleanResources {
  rm -rf ${HostWorkMountDir}/serviceaccount
}

function cleanTunnelServerIPTablesRules {
  iptables-save | grep -v "TUNNEL-PORT" | iptables-restore
}

if [ $NODE_NAME == '' ]; then
  NODE_NAME=$(hostname)
fi

prepareResources

nsenter -t 1 -m -u -n -i sh <<EOF
if [ ! -d /var/run/secrets/kubernetes.io ]; then
  mkdir -p /var/run/secrets/kubernetes.io
fi
cp -a ${HostWorkDir}/serviceaccount /var/run/secrets/kubernetes.io/
${HostWorkDir}/edgectl revert --node-name $NODE_NAME --force
rm -rf /var/run/secrets/kubernetes.io/serviceaccount
exit
EOF

cleanResources

cleanTunnelServerIPTablesRules
