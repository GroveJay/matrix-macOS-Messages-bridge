FROM golang:1-alpine3.21
RUN apk add --no-cache git ca-certificates build-base olm-dev ffmpeg su-exec ca-certificates bash jq curl yq-go

ENV UID=1337 \
    GID=1337 \
    BRIDGEV2=1

COPY . /build
WORKDIR /build

RUN ./build.sh

RUN cp ./bridge /usr/bin/bridge
RUN cp ./docker-run.sh /docker-run.sh
VOLUME /data

CMD ["/docker-run.sh"]