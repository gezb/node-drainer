# node-drainer

This kubernetes controller will drain nodes specified by the NodeDrain CR

```yaml
apiVersion: k8s.gezb.co.uk/v1
kind: NodeDrain
metadata:
  name: nodedrain-node1
spec:
  nodeName: node1
  waitForPodsToRestart: true # optional
```

The controller includes another CR DrainCheck which will cause the controller to check there are no pods that match the given regex before allowing draining

```yaml
apiVersion: k8s.gezb.co.uk/v1
kind: DrainCheck
metadata:
  name: draincheck-blocking
spec:
  podregex: ^blocking-.*
```