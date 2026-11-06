// DTO renvoyés par l'API Go (server/internal/api). Toute modification doit rester alignée
// avec les structs Go : le navigateur ne reçoit jamais de secret, seulement des valeurs masquées.

export type ServiceStatus = 'online' | 'degraded' | 'down' | 'unknown' | 'config_required';

export interface ServiceSummary {
	slug: string;
	name: string;
	monogram: string;
	/** Référence d'une icône selfh.st embarquée ; null : monogramme. */
	icon: string | null;
	color: string;
	category: string;
	hostId: string;
	status: ServiceStatus;
	latencyMs: number | null;
	note: string;
	favorite: boolean;
	/** L'adresse à ouvrir, une fois donnée par l'utilisateur. */
	url?: string | null;
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
	/** Le service qui les fournit, dès qu'une vraie connexion les lit. */
	sessionsSlug?: string;
	favorites: ServiceSummary[];
	/** Tous les services tant qu'aucun favori n'est choisi, sinon vide. */
	services: ServiceSummary[];
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
	/** Le reste n'existe que pour un service découvert. */
	discovered?: boolean;
	hidden?: boolean;
	ports?: number[];
	urlCheck?: UrlCheck;
	project?: ProjectRef;
	links?: ProjectRef[];
	linkable?: ProjectRef[];
	widget?: WidgetInfo;
}

export type WidgetSupport = 'ready' | 'planned' | 'link';

export type WidgetReadState = 'ok' | 'auth' | 'cert' | 'unreachable' | 'unexpected';

/** Les deux derniers sont décidés par le serveur d'après le service tel qu'il est maintenant. */
export type WidgetState = WidgetReadState | 'stopped' | 'address_changed';

export interface WidgetManifest {
	kind: string;
	name: string;
	keyHelp: string;
	/** Les requêtes du connecteur, et rien d'autre. */
	reads: string[];
	widgets: { id: string; label: string; where: string }[];
}

export interface WidgetOption {
	id: string;
	label: string;
	choices: { value: string; label: string }[];
}

/** Jamais la clé : seulement ses derniers caractères. */
export interface WidgetConnection {
	state: WidgetState;
	keyHint: string;
	refreshSeconds: number;
	enabled: string[];
	options: Record<string, string>;
	dataAt: string | null;
	checkedAt: string | null;
	failingSince: string | null;
	data: {
		sessions?: Session[];
		latest?: MediaItem[];
		library?: { movies: number; series: number; episodes: number };
	};
}

export interface MediaItem {
	title: string;
	detail: string;
	kind: 'episode' | 'movie';
	addedAt: string;
}

export interface WidgetInfo {
	support: WidgetSupport;
	manifest?: WidgetManifest;
	refreshChoices?: number[];
	connection?: WidgetConnection;
}

export interface WidgetRow {
	slug: string;
	name: string;
	monogram: string;
	icon: string | null;
	color: string;
	hostId: string;
	hasUrl: boolean;
	support: WidgetSupport;
	/** Vide tant que le service n'est pas connecté. */
	state: WidgetState | '';
	dataAt: string | null;
	failingSince: string | null;
}

export interface WidgetTest {
	result: WidgetReadState;
	latencyMs: number;
	note?: string;
	options?: WidgetOption[];
}

export interface WidgetSave {
	/** Vide : la clé enregistrée reste. */
	apiKey: string;
	widgets: string[];
	refreshSeconds: number;
	options: Record<string, string>;
}

export interface WidgetSaved {
	saved: boolean;
	result: WidgetReadState;
	service?: ServiceDetail;
}

export type UrlResult = 'ok' | 'auth' | 'cert' | 'unreachable' | 'error';

export interface UrlCheck {
	result: UrlResult;
	status: number;
	latencyMs: number;
	checkedAt: string | null;
	certPinned: boolean;
}

export interface UrlCheckResponse {
	saved: boolean;
	result: UrlResult;
	status: number;
	latencyMs: number;
	certSha256?: string;
	certName?: string;
}

export interface ProjectRef {
	key: string;
	containers: number;
}

export interface ServiceEdit {
	name: string;
	category: string;
	icon: string;
	hidden: boolean;
	links: string[];
}

export interface HiddenGroup {
	hostId: string;
	key: string;
	reason: 'agent' | 'database' | 'self' | 'user' | 'service';
	containers: number;
}

/** Décidé par le serveur, pour que tous les écrans s'accordent sur « agent parti ». */
export type AgentState =
	'none' | 'pending' | 'connected' | 'docker_down' | 'offline' | 'expired' | 'revoked';

/** Dates ISO : l'UI écrit elle-même « il y a 8 s » et le tient à jour. */
export interface AgentHost {
	id: string;
	name: string;
	os: string;
	arch: string;
	state: AgentState;
	agentVersion: string;
	dockerVersion: string;
	lastSeenAt: string | null;
	certExpiresAt: string | null;
	enrollmentExpiresAt: string | null;
	/** Conteneurs présents ; les absents restent en base. */
	containers: number;
}

export interface HostsResponse {
	/** Faux tant que le port agents est éteint (pas de HUB_AGENT_CA_FILE). */
	agentsEnabled: boolean;
	serverUrl: string;
	pin: string;
	hosts: AgentHost[];
}

/** Sans valeurs de labels : seulement combien, et combien masqués sur l'hôte. */
export interface Container {
	id: string;
	name: string;
	image: string;
	state: string;
	status: string;
	composeProject: string;
	composeService: string;
	composeDir: string;
	labels: number;
	maskedLabels: number;
}

/** Le code n'est montré qu'une fois : le serveur n'en garde que l'empreinte. */
export interface Enrollment {
	hostId: string;
	code: string;
	expiresAt: string;
	pin: string;
	serverUrl: string;
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
