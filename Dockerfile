
FROM alpine:3.22 AS certs

RUN apk add -U --no-cache ca-certificates

FROM golang:1.26.0 AS build

WORKDIR /work

COPY go.mod* go.sum* ./

RUN go mod download

COPY . .

ENV GOFLAGS=-buildvcs=false

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /build-out/ .

RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && git clone --depth 1 https://github.com/kubernetes-sigs/aws-iam-authenticator.git /aws-iam-authenticator-src \
 && cd /aws-iam-authenticator-src/cmd/aws-iam-authenticator \
 && CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /aws-iam-authenticator

FROM scratch

COPY --from=build /aws-iam-authenticator /usr/bin/

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=build /build-out/* /usr/bin/

WORKDIR /cloudbees/home

ENV HOME=/cloudbees/home
ENV PATH=/usr/bin

ENTRYPOINT ["configure-eks-credentials"]
