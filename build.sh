#!/usr/bin/bash

podman build -t helm-scraper:latest .
podman tag helm-scraper:latest 192.168.1.148:5000/helm-scraper:latest
podman push 192.168.1.148:5000/helm-scraper:latest