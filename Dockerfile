FROM alpine:3.22 AS certs

RUN apk add -U --no-cache ca-certificates

FROM golang:1.26.0 AS build

WORKDIR /work

COPY go.mod* go.sum* ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /build-out/ .

RUN CGO_ENABLED=0 GOOS=linux go install sigs.k8s.io/aws-iam-authenticator/cmd/aws-iam-authenticator@v0.7.2 \
 && mv "$(go env GOPATH)"/bin/aws-iam-authenticator /aws-iam-authenticator

FROM scratch

COPY --from=build /aws-iam-authenticator /usr/bin/

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=build /build-out/* /usr/bin/

WORKDIR /cloudbees/home

ENV HOME=/cloudbees/home
ENV PATH=/usr/bin

ENTRYPOINT ["configure-eks-credentials"]