# Deployer Operator

[![Build](http://prow.purple-chesterfield.com/badge.svg?jobs=image-ham-resource-discoverer-amd64-postsubmit)](http://prow.purple-chesterfield.com/?job=image-ham-resource-discoverer-amd64-postsubmit)
[![GoDoc](https://godoc.org/github.com/hybridapp-io/ham-resource-discoverer?status.svg)](https://godoc.org/github.com/hybridapp-io/ham-resource-discoverer)
[![Go Report Card](https://goreportcard.com/badge/github.com/hybridapp-io/ham-resource-discoverer)](https://goreportcard.com/report/github.com/hybridapp-io/ham-resource-discoverer)
[![Code Coverage](https://codecov.io/gh/hybridapp-io/ham-resource-discoverer/branch/master/graphs/badge.svg?branch=master)](https://codecov.io/gh/hybridapp-io/ham-resource-discoverer?branch=master)
[![License](https://img.shields.io/:license-apache-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)
[![Image](https://quay.io/repository/hybridappio/ham-resource-discoverer/status)](https://quay.io/repository/hybridappio/ham-resource-discoverer?tab=tags)

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [What is the Hybrid Deployable Operator](#what-is-the-hybrid-deployable-operator)
- [Community, discussion, contribution, and support](#community-discussion-contribution-and-support)
- [Getting Started](#getting-started)
    - [Prerequisites](#prerequisites)
    - [Quick Start](#quick-start)
    - [Troubleshooting](#troubleshooting)
- [Hybrid Application References](#hybrid-application-references)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

Deployer Operator

## What is the Deployer Operator

Works with Virtualization Operators (KubeVirt, CloudForms, etc) in a managed cluster for deployments and day2 actions.

## Community, discussion, contribution, and support

Check the [CONTRIBUTING Doc](CONTRIBUTING.md) for how to contribute to the repo.

------

## Getting Started

### Prerequisites

- git v2.18+
- Go v1.13.4+
- operator-sdk v0.18.0
- Kubernetes v1.14+
- kubectl v1.14+

Check the [Development Doc](docs/development.md) for how to contribute to the repo.

### Quick Start

Ideally, 2 kubernetes clusters should be used. 1 for the hub, and 1 for the managed cluster. For this quick start, the  same cluster is for the hub and managed cluster. In this example, the managed cluster toronto in namespace toronto is actually pointing to the hub cluster.

#### Clone Deployer Operator Repository

```shell
mkdir -p "$GOPATH"/src/github.com/hybridapp-io
cd "$GOPATH"/src/github.com/hybridapp-io
git clone https://github.com:hybridapp-io/ham-resource-discoverer.git
cd "$GOPATH"/src/github.com/hybridapp-io/ham-resource-discoverer
```

#### Build Deployer Operator

Build the ham-resource-discoverer and push it to a registry.  Modify the example below to reference a container reposistory you have access to, and update cluster name and it's namespace.

```shell
operator-sdk build quay.io/johndoe/ham-resource-discoverer:v0.1.0
sed -i 's|REPLACE_IMAGE|quay.io/johndoe/ham-resource-discoverer:v0.1.0|g' deploy/operator.yaml
sed -i 's|CLUSTER_NAME|toronto|g' deploy/operator.yaml
sed -i 's|CLUSTER_NAMESPACE|toronto|g' deploy/operator.yaml
docker push quay.io/johndoe/hybriddeployable-operator:v0.1.0
```

#### Install Deployer Operator

Register the ham-resource-discoverer CRDs.

```shell
kubectl create -f deploy/crds
```

Register the ham-resource-discoverer CRD dependencies.

```shell
kubectl create -f hack/test/app_v1alpha1_deployable_crd.yaml
kubectl create -f hack/test/app_v1beta1_application.yaml
kubectl create -f hack/test/cluster-registry-crd.yaml
kubectl create -f hack/test/clusterstatus-crd.yaml
```

Setup RBAC and deploy.

```shell
kubectl create -f deploy/service_account.yaml
kubectl create -f deploy/role.yaml
kubectl create -f deploy/role_binding.yaml
kubectl create -f deploy/operator.yaml
```

Alternatively, the operator can run locally using the operator-sdk, using the following steps:

- Start deployer operator
- kubeconfig points to a kubernetes cluster with admin priviliege
    - **Note:** if you actually have separate hub and managed clusters,
        - use environment variable HUBCLUSTERCONFIGFILE to point to the kubeconfig to hub cluster
        - environment variable KUBECONFIG to point to the kubeconfig to managed cluster

```shell
cd "$GOPATH"/src/github.com/hybridapp-io/ham-resource-discoverer
export CLUSTERNAME=toronto
export CLUSTERNAMESPACE=toronto
operator-sdk run --local --namespace=""
```

Verify ham-resource-discoverer is up and running.

```shell
kubectl get deployment

```

#### Test Deployer Operator

Create the following CRs to setup the test. This will create a Deployer to claim capability in managed cluster.  After Deployer is created, DeployerSet in cluster namespace is updated.

```shell
% kubectl apply -f ./examples/kubevirt.yaml
deployer.app.ibm.com/mykv created
% kubectl get deployerset --all-namespaces
NAMESPACE   NAME      AGE
toronto     toronto   7s
% kubectl describe deployerset -n toronto toronto
Name:         toronto
Namespace:    toronto
Labels:       <none>
Annotations:  <none>
API Version:  app.ibm.com/v1alpha1
Kind:         DeployerSet
Metadata:
  Creation Timestamp:  2020-02-03T22:50:56Z
  Generation:          1
  Resource Version:    12413955
  Self Link:           /apis/app.ibm.com/v1alpha1/namespaces/toronto/deployersets/toronto
  UID:                 a3a9e3af-46d7-11ea-9dba-00000a101bb8
Spec:
  Deployers:
    Key:  default/mykv
    Spec:
      Type:  kubevirt
Events:      <none>
```

Add/set default Deployer (with discovery capability).

```shell
$ kubectl apply -f ./examples/cloudform.yaml
deployer.app.ibm.com/mycf created
$ kubectl describe deployerset -n toronto toronto
Name:         toronto
Namespace:    toronto
Labels:       <none>
Annotations:  <none>
API Version:  app.ibm.com/v1alpha1
Kind:         DeployerSet
Metadata:
  Creation Timestamp:  2020-02-03T22:50:56Z
  Generation:          2
  Resource Version:    12415203
  Self Link:           /apis/app.ibm.com/v1alpha1/namespaces/toronto/deployersets/toronto
  UID:                 a3a9e3af-46d7-11ea-9dba-00000a101bb8
Spec:
  Default Deployer:  default/mycf
  Deployers:
    Key:  default/mycf
    Spec:
      Discovery:
        Gvrs:
          Group:     cloudform.ibm.com
          Resource:  virtualmachines
          Version:   v1alpha1
      Type:          cloudform
    Key:             default/mykv
    Spec:
      Type:  kubevirt
Events:      <none>
$ kubectl get virtualmachines
NAME       AGE
frontend   23m
$ kubectl get deployables -n toronto
NAME                               TEMPLATE-KIND    TEMPLATE-APIVERSION          AGE     STATUS
cloudform-default-frontend-xlclx   VirtualMachine   cloudform.ibm.com/v1alpha1   3m40s
```

## Deploy Workloads

Go to [HybridDeployable repository](https://github.com/IBM/hybriddeployable-operator) to play with workloads.

### Troubleshooting

Please refer to [Trouble shooting documentation](docs/trouble_shooting.md) for further info.
