#!/usr/bin/bash

kubectl apply -f kubernetes/sa.yaml
kubectl apply -f kubernetes/role.yaml
kubectl apply -f kubernetes/binding.yaml
kubectl apply -f kubernetes/clusterrole.yaml
kubectl apply -f kubernetes/clusterrolebinding.yaml
kubectl apply -f kubernetes/cronjob.yaml
