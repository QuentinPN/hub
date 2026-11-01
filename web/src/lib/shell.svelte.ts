/** État partagé entre les pages et la barre du haut, rendue une seule fois par le layout. */
export const shell = $state<{ summary: { online: number; total: number } | null }>({
	summary: null
});
