# node-taint-controller

A Kubernetes controller that applies the `node.kubernetes.io/out-of-service` taint to nodes that have been `NotReady` for a specified duration.

Useful when a node suddenly goes offline due to power outage etc. and needs to be quickly identified and removed from the cluster to prevent pods from being stuck in `Terminating` state.

## Installation

Images are available at `ghcr.io/dcelasun/node-taint-controller`.

Copy the manifest and customize it to your needs:
```sh
$ cp manifest.example.yaml manifest.yaml
# Edit manifest.yaml to adjust thresholds, replicas, etc.
$ kubectl apply -f manifest.yaml
```

## Configuration

```sh
$ make build
$ ./controller --help
Usage of ./controller:
  -health-probe-bind-address string
    	The address the probe endpoint binds to. (default ":8081")
  -kubeconfig string
    	Paths to a kubeconfig. Only required if out-of-cluster.
  -leader-election-id string
    	The ID of the leader election. (default "node-taint-controller.example.com")
  -metrics-bind-address string
    	The address the metric endpoint binds to. (default ":8080")
  -not-ready-threshold duration
    	Duration a node must be NotReady before tainting. (default 5m0s)
  -reconcile-interval duration
    	How often to re-check node status. (default 30s)
  -zap-devel
    	Development Mode defaults(encoder=consoleEncoder,logLevel=Debug,stackTraceLevel=Warn). Production Mode defaults(encoder=jsonEncoder,logLevel=Info,stackTraceLevel=Error)
  -zap-encoder value
    	Zap log encoding (one of 'json' or 'console')
  -zap-log-level value
    	Zap Level to configure the verbosity of logging. Can be one of 'debug', 'info', 'error', 'panic'or any integer value > 0 which corresponds to custom debug levels of increasing verbosity
  -zap-stacktrace-level value
    	Zap Level at and above which stacktraces are captured (one of 'info', 'error', 'panic').
  -zap-time-encoding value
    	Zap time encoding (one of 'epoch', 'millis', 'nano', 'iso8601', 'rfc3339' or 'rfc3339nano'). Defaults to 'epoch'.

```