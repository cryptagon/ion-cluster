FROM golang:1.14-alpine as base 

ENV GO111MODULE=on

WORKDIR $GOPATH/src/github.com/pion/ion-cluster

RUN apk add \
    gstreamer \
    gstreamer-dev \
    gst-plugins-base \
    gst-plugins-base-dev \
    gst-plugins-good \
    gst-plugins-bad \
    gst-plugins-ugly

FROM base as build

RUN apk add \
    build-base \
    pkgconfig

COPY go.mod go.sum ./
RUN cd $GOPATH/src/github.com/pion/ion-cluster && go mod download

COPY . $GOPATH/src/github.com/pion/ion-cluster
RUN GOOS=linux go build -o /ion-cluster .

FROM base 
RUN apk --no-cache add ca-certificates
COPY --from=build /ion-cluster /usr/local/bin/ion-cluster

# COPY config.toml /configs/sfu.toml

ENTRYPOINT ["/usr/local/bin/ion-cluster"]
CMD ["-c", "/configs/sfu.toml", "server"]
