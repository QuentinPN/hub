// Pont entre les options JSON du serveur (go-webauthn) et navigator.credentials. Les
// navigateurs récents savent convertir eux-mêmes (parse…FromJSON, toJSON) ; sinon on
// convertit à la main les seuls champs binaires, encodés en base64url.

/** Ce que renvoient les routes …/begin : les options sous `publicKey`. */
export interface PasskeyOptions<T> {
	publicKey: T;
}

type Json = Record<string, unknown>;

export function base64urlToBytes(s: string): Uint8Array<ArrayBuffer> {
	const b64 = s.replace(/-/g, '+').replace(/_/g, '/');
	const bin = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4));
	return Uint8Array.from(bin, (c) => c.charCodeAt(0));
}

export function bytesToBase64url(buf: ArrayBuffer | ArrayBufferView): string {
	const bytes =
		buf instanceof ArrayBuffer
			? new Uint8Array(buf)
			: new Uint8Array(buf.buffer, buf.byteOffset, buf.byteLength);
	let bin = '';
	for (const b of bytes) bin += String.fromCharCode(b);
	return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

const decodeIds = (list: unknown) =>
	Array.isArray(list)
		? list.map((d: Json) => ({ ...d, id: base64urlToBytes(d.id as string) }))
		: undefined;

/** Conversion manuelle des options d'enregistrement (challenge, user.id, exclusions). */
export function creationOptionsFromJSON(json: Json): PublicKeyCredentialCreationOptions {
	const user = json.user as Json;
	return {
		...json,
		challenge: base64urlToBytes(json.challenge as string),
		user: { ...user, id: base64urlToBytes(user.id as string) },
		excludeCredentials: decodeIds(json.excludeCredentials)
	} as unknown as PublicKeyCredentialCreationOptions;
}

/** Conversion manuelle des options de connexion (challenge, clés autorisées). */
export function requestOptionsFromJSON(json: Json): PublicKeyCredentialRequestOptions {
	return {
		...json,
		challenge: base64urlToBytes(json.challenge as string),
		allowCredentials: decodeIds(json.allowCredentials)
	} as unknown as PublicKeyCredentialRequestOptions;
}

/** Réponse de l'authentificateur au format JSON attendu par le serveur (PublicKeyCredential.toJSON). */
export function credentialToJSON(cred: PublicKeyCredential): Json {
	if (typeof cred.toJSON === 'function') return cred.toJSON() as unknown as Json;
	const r = cred.response;
	const response: Json = { clientDataJSON: bytesToBase64url(r.clientDataJSON) };
	if (r instanceof AuthenticatorAttestationResponse) {
		response.attestationObject = bytesToBase64url(r.attestationObject);
		response.transports = r.getTransports?.() ?? [];
	} else {
		const a = r as AuthenticatorAssertionResponse;
		response.authenticatorData = bytesToBase64url(a.authenticatorData);
		response.signature = bytesToBase64url(a.signature);
		if (a.userHandle) response.userHandle = bytesToBase64url(a.userHandle);
	}
	return {
		id: cred.id,
		rawId: bytesToBase64url(cred.rawId),
		type: cred.type,
		authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
		response,
		clientExtensionResults: cred.getClientExtensionResults()
	};
}

/** Le navigateur sait-il utiliser les passkeys ici ? (Il faut aussi un contexte sécurisé.) */
export const passkeysSupported = () =>
	typeof window !== 'undefined' && window.isSecureContext && 'PublicKeyCredential' in window;

/** Crée une passkey (Windows Hello, Touch ID, téléphone…) à partir des options du serveur. */
export async function createPasskey(options: PasskeyOptions<Json>): Promise<Json> {
	const PKC = PublicKeyCredential as typeof PublicKeyCredential & {
		parseCreationOptionsFromJSON?: (o: unknown) => PublicKeyCredentialCreationOptions;
	};
	const publicKey = PKC.parseCreationOptionsFromJSON
		? PKC.parseCreationOptionsFromJSON(options.publicKey)
		: creationOptionsFromJSON(options.publicKey);
	const cred = await navigator.credentials.create({ publicKey });
	if (!(cred instanceof PublicKeyCredential))
		throw new DOMException('no credential', 'NotAllowedError');
	return credentialToJSON(cred);
}

/** Demande une passkey existante pour ce site, sans identifiant (connexion « découvrable »). */
export async function getPasskey(options: PasskeyOptions<Json>): Promise<Json> {
	const PKC = PublicKeyCredential as typeof PublicKeyCredential & {
		parseRequestOptionsFromJSON?: (o: unknown) => PublicKeyCredentialRequestOptions;
	};
	const publicKey = PKC.parseRequestOptionsFromJSON
		? PKC.parseRequestOptionsFromJSON(options.publicKey)
		: requestOptionsFromJSON(options.publicKey);
	const cred = await navigator.credentials.get({ publicKey });
	if (!(cred instanceof PublicKeyCredential))
		throw new DOMException('no credential', 'NotAllowedError');
	return credentialToJSON(cred);
}

/** Vrai quand l'utilisateur a fermé ou laissé expirer la fenêtre du système : rien à signaler. */
export const isCancelled = (err: unknown) =>
	err instanceof DOMException && (err.name === 'NotAllowedError' || err.name === 'AbortError');
