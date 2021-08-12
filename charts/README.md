# OpenYurt Operator

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

### Install OpenYurt Operator

```shell
# helm install openyurt-operator ./charts -n kube-system
```

### Convert the cluster to edge (apply YurtCluster)

```shell
# kubectl apply -f ./config/samples/operator_v1alpha1_yurtcluster.yaml
```

### Convert a Node to Edge Node

```shell
# kubectl label node <NODE_NAME> openyurt.io/is-edge-worker=true --overwrite
```

### Revert a Node to Normal Node

```shell
# kubectl label node <NODE_NAME> openyurt.io/is-edge-worker=false --overwrite
```

### Revert the cluster to normal

```shell
# kubectl delete yurtclusters openyurt
```
