import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const path = resolve(root, "keycloak/kalke-realm.json");
const realm = JSON.parse(readFileSync(path, "utf8"));

const requiredClients = [
	"personal-document-extractor",
	"e-bank-api",
	"kalke-spa",
	"kalke-cli",
	"pde-m2m",
	"ebank-m2m",
];
const requiredRoles = ["extract:write", "bank:write", "admin"];

const clientIds = (realm.clients || []).map((c) => c.clientId);
const roleNames = (realm.roles?.realm || []).map((r) => r.name);

const missingClients = requiredClients.filter((id) => !clientIds.includes(id));
const missingRoles = requiredRoles.filter((name) => !roleNames.includes(name));

const spa = (realm.clients || []).find((c) => c.clientId === "kalke-spa");
const hasProdRedirect =
	spa?.redirectUris?.some((u) => String(u).includes("kalke.dev")) ?? false;

const errors = [];
if (realm.realm !== "kalke") errors.push(`unexpected realm name: ${realm.realm}`);
if (missingClients.length) errors.push(`missing clients: ${missingClients.join(", ")}`);
if (missingRoles.length) errors.push(`missing roles: ${missingRoles.join(", ")}`);
if (!hasProdRedirect) errors.push("kalke-spa missing https://kalke.dev redirect");

if (errors.length) {
	console.error("Realm validation failed:");
	for (const e of errors) console.error(`  - ${e}`);
	process.exit(1);
}

console.log("Realm OK:", {
	clients: clientIds.length,
	roles: roleNames.length,
	spaRedirects: spa.redirectUris.length,
});
