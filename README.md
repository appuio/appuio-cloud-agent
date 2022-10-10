# APPUiO Cloud Agent

[![Build](https://img.shields.io/github/workflow/status/appuio/appuio-cloud-agent/Test)][build]
![Go version](https://img.shields.io/github/go-mod/go-version/appuio/appuio-cloud-agent)
[![Version](https://img.shields.io/github/v/release/appuio/appuio-cloud-agent)][releases]
[![Maintainability](https://img.shields.io/codeclimate/maintainability/appuio/appuio-cloud-agent)][codeclimate]
[![Coverage](https://img.shields.io/codeclimate/coverage/appuio/appuio-cloud-agent)][codeclimate]
[![GitHub downloads](https://img.shields.io/github/downloads/appuio/appuio-cloud-agent/total)][releases]

[build]: https://github.com/appuio/appuio-cloud-agent/actions?query=workflow%3ATest
[releases]: https://github.com/appuio/appuio-cloud-agent/releases
[codeclimate]: https://codeclimate.com/github/appuio/appuio-cloud-agent

The APPUiO Cloud Agent is a controller running on every APPUiO Cloud Zone.



## Run locally

## Local development environment

You can setup a [kind]-based local environment with

```bash
make kind
export KUBECONFIG=.kind/kind-kubeconfig-v1.23.0
```

[kind]: https://kind.sigs.k8s.io/


### Running the agent locally

You can run the agent locally against the currently configured Kubernetes cluster with

```bash
make run
```

To access the locally running controller webhook server, you need to register it with the [kind]-based local environment.
You can do this by applying the following manifests:

```
HOSTIP=$(docker inspect appuio-cloud-agent-v1.23.0-control-plane | jq '.[0].NetworkSettings.Networks.kind.Gateway')

# Under Docker for Mac `docker.for.mac.localhost` can be used instead of the host IP.
# HOSTIP=docker.for.mac.localhost

cat <<EOF | sed -e "s/172.21.0.1/$HOSTIP/g" | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: webhook-service
  namespace: default
spec:
  ports:
  - port: 9443
    protocol: TCP
    targetPort: 9443
  type: ExternalName
  externalName: 172.21.0.1 # Change to host IP
EOF

kubectl apply -f ./config/webhook/manifests.yaml

# On BSD/MacOS base64 must be called without `-w0`
cert="$(base64 ./webhook-certs/tls.crt)"

patch_tmpl='.webhooks | keys[] as $i | [
    {
      "op": "add",
      "path": "/webhooks/\($i)/clientConfig/caBundle",
      "value": "'"$cert"'"
    },
    {
      "op": "replace",
      "path": "/webhooks/\($i)/clientConfig/service/namespace",
      "value": "default"
    },
    {
      "op": "replace",
      "path": "/webhooks/\($i)/clientConfig/service/port",
      "value": 9443
    }
  ]'

patches=$(kubectl get validatingwebhookconfiguration validating-webhook-configuration -ojson \
  | jq -rc "$patch_tmpl")
while read -r patch; do
   kubectl patch validatingwebhookconfiguration validating-webhook-configuration --type=json -p "${patch}"
done <<< "$patches"

patches=$(kubectl get mutatingwebhookconfigurations mutating-webhook-configuration -ojson \
  | jq -rc "$patch_tmpl")
while read -r patch; do
   kubectl patch mutatingwebhookconfigurations mutating-webhook-configuration --type=json -p "${patch}"
done <<< "$patches"
```
