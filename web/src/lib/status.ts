import type { ServiceStatus } from './api/types';

interface StatusStyle {
	label: string;
	color: string;
}

// « inconnu » est gris, jamais rouge : un agent muet n'est pas une panne du service.
const styles: Record<ServiceStatus, StatusStyle> = {
	online: { label: 'En ligne', color: 'var(--status-online)' },
	degraded: { label: 'Dégradé', color: 'var(--status-degraded)' },
	down: { label: 'Arrêté', color: 'var(--status-down)' },
	unknown: { label: 'Inconnu', color: 'var(--status-unknown)' },
	config_required: { label: 'Configuration requise', color: 'var(--status-unknown)' }
};

export const statusStyle = (status: ServiceStatus): StatusStyle => styles[status];
