export IMAGE=quay.io/rbmateescu/ham-resource-discoverer:v0.0.1
oc login https://api.foil.os.fyre.ibm.com:6443 -u admin -p Passw0rd --insecure-skip-tls-verify=true
oc project ham
oc project ham
kubectl delete -f deploy/foil
operator-sdk build $IMAGE
docker push $IMAGE
kubectl apply -f deploy/foil
oc project wordpress-02
sleep 20s
kubectl logs `kubectl get pods -n ham | grep ham-resource-discoverer | head -n1 | awk '{print $1;}'` -n ham -f
