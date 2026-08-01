FROM quay.io/keycloak/keycloak:26.3

COPY keycloak/kalke-realm.json /opt/keycloak/data/import/kalke-realm.json

ENV KC_HTTP_ENABLED=true \
	KC_HOSTNAME_STRICT=false \
	KC_PROXY_HEADERS=xforwarded \
	KC_HEALTH_ENABLED=true \
	KC_HTTP_PORT=8080

EXPOSE 8080

ENTRYPOINT ["/opt/keycloak/bin/kc.sh"]
CMD ["start", "--import-realm"]
