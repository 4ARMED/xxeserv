FROM golang:alpine AS build

ENV XXESERV $GOPATH/src/github.com/4armed/xxeserv

RUN mkdir -p /out/

WORKDIR $XXESERV
COPY xxeftp.go .

RUN GOOS=linux CGO_ENABLED=0 go build -a -installsuffix "static" -o xxeftp && cp ./xxeftp /out/xxeftp

FROM scratch
LABEL maintainer="Marc Wickenden <marc@4armed.com>"
COPY --from=build /out/xxeftp /xxeftp
ENTRYPOINT ["/xxeftp"]
CMD [ "-p", "2121" ]