import { describe, expect, it } from 'vitest';
import {
	base64urlToBytes,
	bytesToBase64url,
	creationOptionsFromJSON,
	isCancelled,
	requestOptionsFromJSON
} from './webauthn';

describe('base64url', () => {
	it('fait l’aller-retour sans remplissage et avec les caractères - et _', () => {
		const bytes = Uint8Array.from([0xfb, 0xff, 0xfe, 0x00, 0x01]);
		const s = bytesToBase64url(bytes);
		expect(s).toBe('-__-AAE');
		expect([...base64urlToBytes(s)]).toEqual([...bytes]);
		expect(bytesToBase64url(bytes.buffer)).toBe(s);
	});
});

describe('conversion manuelle des options', () => {
	it('décode challenge, user.id et exclusions pour create()', () => {
		const opts = creationOptionsFromJSON({
			challenge: 'AQID',
			rp: { id: 'hub.test', name: 'Hub' },
			user: { id: 'BAU', name: 'quentin', displayName: 'quentin' },
			excludeCredentials: [{ type: 'public-key', id: 'Bgc' }]
		});
		expect([...new Uint8Array(opts.challenge as ArrayBuffer)]).toEqual([1, 2, 3]);
		expect([...new Uint8Array(opts.user.id as ArrayBuffer)]).toEqual([4, 5]);
		expect([
			...new Uint8Array((opts.excludeCredentials?.[0]?.id ?? new ArrayBuffer(0)) as ArrayBuffer)
		]).toEqual([6, 7]);
		expect(opts.rp.id).toBe('hub.test');
	});

	it('laisse allowCredentials absent pour une connexion découvrable', () => {
		const opts = requestOptionsFromJSON({ challenge: 'AQID', userVerification: 'required' });
		expect(opts.allowCredentials).toBeUndefined();
		expect(opts.userVerification).toBe('required');
	});
});

describe('isCancelled', () => {
	it('reconnaît la fenêtre fermée par l’utilisateur', () => {
		expect(isCancelled(new DOMException('x', 'NotAllowedError'))).toBe(true);
		expect(isCancelled(new DOMException('x', 'InvalidStateError'))).toBe(false);
		expect(isCancelled(new Error('x'))).toBe(false);
	});
});
