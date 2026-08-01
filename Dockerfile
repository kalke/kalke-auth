FROM quay.io/keycloak/keycloak:26.3

# Production import: no demo users, no password-grant CLI, no committed client secrets.
COPY keycloak/kalke-realm.prod.json /opt/keycloak/data/import/kalke-realm.json

ENV KC_HTTP_ENABLED=true \
	KC_HOSTNAME_STRICT=true \
	KC_PROXY_HEADERS=xforwarded \
	KC_HEALTH_ENABLED=true \
	KC_HTTP_PORT=8080

EXPOSE 8080

ENTRYPOINT ["/opt/keycloak/bin/kc.sh"]
CMD ["start", "--import-realm"]
