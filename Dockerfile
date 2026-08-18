FROM golang:1.26.6-bookworm AS api-build
WORKDIR /src
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/loadsecret ./cmd/loadsecret

FROM quay.io/keycloak/keycloak:26.7
USER root

COPY --from=api-build /out/api /opt/kalke/api
COPY --from=api-build /out/loadsecret /opt/kalke/loadsecret
COPY keycloak/kalke-realm.prod.json /opt/keycloak/data/import/kalke-realm.json
COPY scripts/entrypoint.sh /opt/kalke/entrypoint.sh
RUN chmod 755 /opt/kalke/entrypoint.sh /opt/kalke/api /opt/kalke/loadsecret

ENV KC_HTTP_ENABLED=true \
	KC_HOSTNAME_STRICT=true \
	KC_PROXY_HEADERS=xforwarded \
	KC_HEALTH_ENABLED=true \
	KC_HTTP_PORT=8081 \
	HTTP_ADDR=:8080 \
	KC_INTERNAL_URL=http://127.0.0.1:8081

EXPOSE 8080
USER keycloak
ENTRYPOINT ["/opt/kalke/entrypoint.sh"]
