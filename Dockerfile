FROM alpine:3.18 as certs

RUN apk add -U --no-cache ca-certificates

FROM golang:1.20.5-alpine3.18 AS build

WORKDIR /work

COPY go.mod* go.sum* ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /build-out/ .

FROM public.ecr.aws/eks-distro/kubernetes-sigs/aws-iam-authenticator:v0.6.12-eks-1-27-latest as awsiamauth

FROM scratch

COPY --from=awsiamauth /aws-iam-authenticator /usr/bin/

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=build /build-out/* /usr/bin/

WORKDIR /cloudbees/home

ENV HOME=/cloudbees/home
ENV PATH=/usr/bin

ENTRYPOINT ["configure-eks-credentials"]
