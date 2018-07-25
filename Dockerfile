FROM golang:1.10-alpine as build
WORKDIR $GOPATH/src/github.com/danielgtaylor/apisprout
COPY . .
RUN apk add --no-cache git && \
  go get github.com/ahmetb/govvv && \
  govvv install

FROM alpine:3.8
COPY --from=build /go/bin/apisprout /usr/local/bin/
RUN apk add --no-cache ca-certificates && \
  update-ca-certificates
ENTRYPOINT ["/usr/local/bin/apisprout"]
EXPOSE 8000
