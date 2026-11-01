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
