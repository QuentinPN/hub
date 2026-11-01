// DTO renvoyés par l'API Go (server/internal/api). Toute modification doit rester alignée
// avec les structs Go : le navigateur ne reçoit jamais de secret, seulement des valeurs masquées.

export type ServiceStatus = 'online' | 'degraded' | 'down' | 'unknown' | 'config_required';

export interface ServiceSummary {
	slug: string;
	name: string;
	monogram: string;
	color: string;
	category: string;
	hostId: string;
	status: ServiceStatus;
	latencyMs: number | null;
	note: string;
	favorite: boolean;
}

export interface Session {
	title: string;
	subtitle: string;
	device: string;
	progress: number;
	durationLabel: string;
	transcoding: boolean;
}

export interface UpcomingItem {
	title: string;
	detail: string;
	when: string;
	kind: 'episode' | 'movie';
	tone: string;
}

export interface HostMetric {
	label: string;
	percent: number;
}

export interface Host {
	id: string;
	name: string;
	os: string;
	containers: number;
	reachable: boolean;
	lastSeenLabel: string;
	metrics: HostMetric[];
}

export interface Home {
	summary: { online: number; total: number };
	sessions: Session[];
	favorites: ServiceSummary[];
	upcoming: UpcomingItem[];
	hosts: Host[];
}

export interface KeyValue {
	label: string;
	value: string;
	mono?: boolean;
}

export interface ServiceDetail extends ServiceSummary {
	version: string;
	tint: string;
	stats: { value: string; label: string }[];
	upcoming: UpcomingItem[];
	identification: KeyValue[];
	access: KeyValue[];
	connection: KeyValue[];
}

export interface AgentHost {
	id: string;
	name: string;
	os: string;
	arch: string;
	containers: number;
	reachable: boolean;
	lastSeenLabel: string;
	agentVersion: string;
	certExpiresLabel: string;
}

export type RemovalPolicy = 'absent_then_archive' | 'absent_only' | 'archive_now';

export interface Discovery {
	followEvents: boolean;
	rescanMinutes: number;
	onRemoval: RemovalPolicy;
	archiveAfterDays: number;
	confirmBelowPercent: number;
	/** Toujours faux : un port publié n'est jamais pris pour une URL utilisable. */
	inferUrlFromPorts: boolean;
}

export interface Settings {
	/** Faux tant que les réglages ne sont pas enregistrés dans PostgreSQL. */
	editable: boolean;
	hosts: AgentHost[];
	discovery: Discovery;
}

export interface AuthState {
	/** Vrai tant qu'aucun compte n'existe : la page de connexion propose alors de le créer. */
	setupRequired: boolean;
	authenticated: boolean;
	username?: string;
	/** Les passkeys sont configurées côté serveur (HUB_ORIGIN) : la connexion les propose. */
	passkeys: boolean;
}

export interface PasskeyInfo {
	id: number;
	name: string;
	createdAt: string;
	lastUsedAt: string | null;
	/** Passkey synchronisée (gestionnaire de mots de passe, trousseau…) plutôt que liée à un appareil. */
	backedUp: boolean;
}

export interface SessionInfo {
	id: number;
	/** La session de ce navigateur. */
	current: boolean;
	userAgent: string;
	ip: string;
	createdAt: string;
	lastSeenAt: string;
}
