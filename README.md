# tyk-sre-assignment

This repository contains the boilerplate projects for the SRE role interview assignments. There are two projects: one for Go and one for Python respectively.

### Go Project

Location: https://github.com/TykTechnologies/tyk-sre-assignment/tree/main/golang

In order to build the project run:
```
go mod tidy & go build
```

To run it against a real Kubernetes API server:
```
./tyk-sre-assignment --kubeconfig '/path/to/your/kube/conf' --address ":8080"

./tyk-sre-assignment --kubeconfig '/home/lordwales/.kube/config' --address ":8080" --source-namespace 'default' --destination-namespace 'tom' --source-selector 'app=nginx' --destination-selector 'app=nginx2'

./tyk-sre-assignment --kubeconfig '/home/lordwales/.kube/config' --address ":8086" --namespace 'tom' --selector 'app=nginx2'

kubectl get networkpolicy isolate-default-tom -n tom -o yaml

docker run -v ~/.kube/config:/kube/config -p 8040:8040 -e KUBECONFIG=/kube/config -e LISTEN_ADDRESS=:8040 -e NAMESPACE=tom -e SELECTOR=app=nginx2 my-tyk:v1
```

To execute unit tests:
```
go test -v
```

### Python Project

Location: https://github.com/TykTechnologies/tyk-sre-assignment/tree/main/python

We suggest using a Python virtual env, e.g.:
```
python3 -m venv .venv
source .venv/bin/activate
```

Make sure to install the dependencies using `pip`:
```
pip3 install -r requirements.txt
```

To run it against a real Kubernetes API server:
```
python3 main.py --kubeconfig '/path/to/your/kube/conf' --address ":8080"
```

To execute unit tests:
```
python3 tests.py -v
```
