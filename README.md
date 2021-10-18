# YurtCluster Operator

## Quick Start

### Prepare a Kubernetes cluster

```shell
# cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
- role: worker
EOF
```

### Install YurtCluster Operator

```shell
# helm install operator ./charts -n kube-system
```

### Convert the cluster to Yurt cluster

```shell
# kubectl apply -f ./config/samples/operator_v1alpha1_yurtcluster.yaml
```

### Convert a Node to Cloud Node

```shell
# kubectl label node <NODE_NAME> openyurt.io/node-type=cloud
```

### Convert a Node to Edge Node

```shell
# kubectl label node <NODE_NAME> openyurt.io/node-type=edge
```


### Revert a Node to Normal Node

```shell
# kubectl label node <NODE_NAME> openyurt.io/node-type-
```

### Revert the cluster to normal

```shell
# kubectl delete yurtclusters cluster
```
