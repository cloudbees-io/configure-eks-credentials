ARG AWS_IAM_AUTHENTICATOR_VERSION=0.7.18

FROM alpine:3.23 AS certs
RUN apk add -U --no-cache ca-certificates

FROM alpine:3.23 AS awsiamauth
ARG AWS_IAM_AUTHENTICATOR_VERSION
WORKDIR /
RUN wget -q https://github.com/kubernetes-sigs/aws-iam-authenticator/releases/download/v${AWS_IAM_AUTHENTICATOR_VERSION}/aws-iam-authenticator_${AWS_IAM_AUTHENTICATOR_VERSION}_linux_amd64 -O aws-iam-authenticator && \
    chmod 755 aws-iam-authenticator

FROM scratch
COPY --from=awsiamauth /aws-iam-authenticator /usr/bin/
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY configure-eks-credentials /usr/bin/
WORKDIR /cloudbees/home
ENV HOME=/cloudbees/home
ENV PATH=/usr/bin

ENTRYPOINT ["configure-eks-credentials"]
