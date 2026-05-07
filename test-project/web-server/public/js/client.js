// gomelo Admin API Client

class AdminClient {
    constructor(baseUrl) {
        this.baseUrl = baseUrl || '';
    }

    async getServers() {
        var res = await fetch(this.baseUrl + '/api/servers');
        return await res.json();
    }

    async getStats() {
        var res = await fetch(this.baseUrl + '/api/stats');
        return await res.json();
    }

    async getConnections() {
        var res = await fetch(this.baseUrl + '/api/connections');
        return await res.json();
    }
}

window.gomeloAdmin = { AdminClient: AdminClient };
