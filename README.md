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

kubctl apply -f ./config/webhook/manifests.yaml

kubectl patch validatingwebhookconfiguration validating-webhook-configuration \
  -p '{
    "webhooks": [
      {
        "name": "validate-users.appuio.io",
        "clientConfig": {
          "caBundle": "'"$(base64 -w0 "./local-env/webhook-certs/tls.crt)"'",
          "service": {
            "namespace": "default",
            "port": 9443
          }
        }
      }
    ]
  }'
```
