// Drives the single New-resource modal hosted in the root layout. Both the top
// bar's "+ New" and a cluster row's "[+]" in the sidebar open it through here, so
// the trigger can live anywhere in the tree without prop-drilling.
type Mode = 'choose' | 'server' | 'cluster';

class NewResourceStore {
	open = $state(false);
	mode = $state<Mode>('choose');
	// When opened from a cluster's [+], the New-Server form preselects + locks this
	// cluster as the membership. null = free choice.
	lockedCluster = $state<string | null>(null);

	// Top-bar "+ New": start at the Server/Cluster chooser.
	openChooser() {
		this.mode = 'choose';
		this.lockedCluster = null;
		this.open = true;
	}

	// Sidebar "[+]" on a cluster: jump straight to New Server, locked to it.
	openServerForCluster(cluster: string) {
		this.mode = 'server';
		this.lockedCluster = cluster;
		this.open = true;
	}

	close() {
		this.open = false;
	}
}

export const newResource = new NewResourceStore();
