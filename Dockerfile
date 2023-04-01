FROM alpine:edge
ENTRYPOINT ["/bin/deltaircd"]

COPY . /go/src/github.com/deltachat/deltaircd
RUN apk update && apk add go git gcc musl-dev ca-certificates \
        && cd /go/src/github.com/deltachat/deltaircd \
        && export GOPATH=/go \
        && go get \
        && go build -x -ldflags "-X main.githash=$(git log --pretty=format:'%h' -n 1)" -o /bin/deltaircd \
        && rm -rf /go \
        && apk del --purge git go gcc musl-dev
