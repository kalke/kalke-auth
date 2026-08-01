import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

function load(rel) {
	return JSON.parse(readFileSync(resolve(root, rel), "utf8"));
}

function fail(msg) {
	console.error(msg);
	process.exitCode = 1;
}

function validateDev(realm) {
	const requiredClients = [
		"personal-document-extractor",
		"e-bank-api",
		"kalke-spa",
		"kalke-cli",
		"pde-m2m",
		"ebank-m2m",
	];
	const clientIds = (realm.clients || []).map((c) => c.clientId);
	for (const id of requiredClients) {
		if (!clientIds.includes(id)) fail(`dev realm missing client: ${id}`);
	}
	const roles = (realm.roles?.realm || []).map((r) => r.name);
	for (const name of ["extract:write", "bank:write", "admin"]) {
		if (!roles.includes(name)) fail(`dev realm missing role: ${name}`);
	}
}

function validateProd(realm) {
	const clientIds = (realm.clients || []).map((c) => c.clientId);
	for (const id of [
		"personal-document-extractor",
		"e-bank-api",
		"kalke-spa",
		"kalke-bff",
		"pde-m2m",
		"ebank-m2m",
	]) {
		if (!clientIds.includes(id)) fail(`prod realm missing client: ${id}`);
	}
	if (clientIds.includes("kalke-cli")) {
		fail("prod realm must not include password-grant client kalke-cli");
	}
	for (const c of realm.clients || []) {
		if (c.secret) fail(`prod client ${c.clientId} must not embed a secret`);
		if (c.directAccessGrantsEnabled && c.clientId !== "kalke-bff") {
			fail(`prod client ${c.clientId} must not enable direct access grants`);
		}
	}
	const bff = (realm.clients || []).find((c) => c.clientId === "kalke-bff");
	if (!bff?.directAccessGrantsEnabled || bff.publicClient) {
		fail("prod kalke-bff must be confidential with direct access grants");
	}
	const spa = (realm.clients || []).find((c) => c.clientId === "kalke-spa");
	const redirects = spa?.redirectUris || [];
	if (redirects.some((u) => String(u).includes("localhost"))) {
		fail("prod kalke-spa must not allow localhost redirects");
	}
	if (!redirects.some((u) => String(u).includes("/playground"))) {
		fail("prod kalke-spa missing /playground redirect");
	}
	const users = realm.users || [];
	for (const u of users) {
		const pw = u.credentials?.find((c) => c.type === "password")?.value;
		if (pw) fail(`prod user ${u.username} must not embed a password`);
	}
}

const dev = load("keycloak/kalke-realm.json");
const prod = load("keycloak/kalke-realm.prod.json");
if (dev.realm !== "kalke" || prod.realm !== "kalke") fail("unexpected realm name");
validateDev(dev);
validateProd(prod);

if (process.exitCode) {
	console.error("Realm validation failed");
	process.exit(process.exitCode);
}
console.log("Realm OK (dev + prod)");
