export namespace models {
	
	export class AccountTestResult {
	    success: boolean;
	    message: string;
	    latencyMs: number;
	    testedService: string;
	    // Go type: time
	    timestamp: any;
	
	    static createFrom(source: any = {}) {
	        return new AccountTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.latencyMs = source["latencyMs"];
	        this.testedService = source["testedService"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AppSettings {
	    gatewayPort: number;
	    gatewayIP: string;
	    isRunning: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gatewayPort = source["gatewayPort"];
	        this.gatewayIP = source["gatewayIP"];
	        this.isRunning = source["isRunning"];
	    }
	}
	export class GatewayStatus {
	    isRunning: boolean;
	    ip: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new GatewayStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isRunning = source["isRunning"];
	        this.ip = source["ip"];
	        this.port = source["port"];
	    }
	}
	export class GoogleAccountResponse {
	    id: string;
	    email: string;
	    name: string;
	    avatarUrl: string;
	    status: string;
	    services: string;
	    tier: string;
	    credits: number;
	    proxy: string;
	    imageEnabled: boolean;
	    videoEnabled: boolean;
	    hasPsid: boolean;
	    hasPsidts: boolean;
	    hasSnlm0e: boolean;
	    cookiePreview: string;
	    lastError?: string;
	    lastRefreshAt?: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new GoogleAccountResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.name = source["name"];
	        this.avatarUrl = source["avatarUrl"];
	        this.status = source["status"];
	        this.services = source["services"];
	        this.tier = source["tier"];
	        this.credits = source["credits"];
	        this.proxy = source["proxy"];
	        this.imageEnabled = source["imageEnabled"];
	        this.videoEnabled = source["videoEnabled"];
	        this.hasPsid = source["hasPsid"];
	        this.hasPsidts = source["hasPsidts"];
	        this.hasSnlm0e = source["hasSnlm0e"];
	        this.cookiePreview = source["cookiePreview"];
	        this.lastError = source["lastError"];
	        this.lastRefreshAt = source["lastRefreshAt"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class LoginSessionStatus {
	    sessionId: string;
	    step: string;
	    message: string;
	    account?: GoogleAccountResponse;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new LoginSessionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.step = source["step"];
	        this.message = source["message"];
	        this.account = this.convertValues(source["account"], GoogleAccountResponse);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UserResponse {
	    id: number;
	    username: string;
	    role: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new UserResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.role = source["role"];
	        this.created_at = source["created_at"];
	    }
	}

}

