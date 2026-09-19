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
	export class BackupExportResult {
	    success: boolean;
	    filePath: string;
	    accountCount: number;
	    profileCount: number;
	    sizeBytes: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new BackupExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.filePath = source["filePath"];
	        this.accountCount = source["accountCount"];
	        this.profileCount = source["profileCount"];
	        this.sizeBytes = source["sizeBytes"];
	        this.message = source["message"];
	    }
	}
	export class BulkAccountItem {
	    email: string;
	    status: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new BulkAccountItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.status = source["status"];
	        this.message = source["message"];
	    }
	}
	export class BulkAddInput {
	    rawList: string;
	    defaultProxy: string;
	    skipExisting: boolean;
	    defaultTier: string;
	    service: string;
	
	    static createFrom(source: any = {}) {
	        return new BulkAddInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rawList = source["rawList"];
	        this.defaultProxy = source["defaultProxy"];
	        this.skipExisting = source["skipExisting"];
	        this.defaultTier = source["defaultTier"];
	        this.service = source["service"];
	    }
	}
	export class BulkAddResult {
	    totalParsed: number;
	    addedCount: number;
	    skippedCount: number;
	    failedCount: number;
	    items: BulkAccountItem[];
	
	    static createFrom(source: any = {}) {
	        return new BulkAddResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalParsed = source["totalParsed"];
	        this.addedCount = source["addedCount"];
	        this.skippedCount = source["skippedCount"];
	        this.failedCount = source["failedCount"];
	        this.items = this.convertValues(source["items"], BulkAccountItem);
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
	export class CachePurgeResult {
	    success: boolean;
	    freedBytes: number;
	    profilesCleaned: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new CachePurgeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.freedBytes = source["freedBytes"];
	        this.profilesCleaned = source["profilesCleaned"];
	        this.message = source["message"];
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
	export class LiveGeminiQuota {
	    available: boolean;
	    tier: string;
	    currentUsedPercent: number;
	    hasCurrentUsedPercent: boolean;
	    currentRemainingPercent: number;
	    hasCurrentRemaining: boolean;
	    currentResetAt?: string;
	    weeklyUsedPercent: number;
	    hasWeeklyUsedPercent: boolean;
	    weeklyRemainingPercent: number;
	    hasWeeklyRemaining: boolean;
	    weeklyResetAt?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new LiveGeminiQuota(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.tier = source["tier"];
	        this.currentUsedPercent = source["currentUsedPercent"];
	        this.hasCurrentUsedPercent = source["hasCurrentUsedPercent"];
	        this.currentRemainingPercent = source["currentRemainingPercent"];
	        this.hasCurrentRemaining = source["hasCurrentRemaining"];
	        this.currentResetAt = source["currentResetAt"];
	        this.weeklyUsedPercent = source["weeklyUsedPercent"];
	        this.hasWeeklyUsedPercent = source["hasWeeklyUsedPercent"];
	        this.weeklyRemainingPercent = source["weeklyRemainingPercent"];
	        this.hasWeeklyRemaining = source["hasWeeklyRemaining"];
	        this.weeklyResetAt = source["weeklyResetAt"];
	        this.message = source["message"];
	    }
	}
	export class LiveFlowQuota {
	    available: boolean;
	    tier: string;
	    totalCredits: number;
	    hasTotalCredits: boolean;
	    dailyCredits: number;
	    hasDailyCredits: boolean;
	    monthlyCredits: number;
	    hasMonthlyCredits: boolean;
	    dailyResetAt?: string;
	    monthlyResetAt?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new LiveFlowQuota(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.tier = source["tier"];
	        this.totalCredits = source["totalCredits"];
	        this.hasTotalCredits = source["hasTotalCredits"];
	        this.dailyCredits = source["dailyCredits"];
	        this.hasDailyCredits = source["hasDailyCredits"];
	        this.monthlyCredits = source["monthlyCredits"];
	        this.hasMonthlyCredits = source["hasMonthlyCredits"];
	        this.dailyResetAt = source["dailyResetAt"];
	        this.monthlyResetAt = source["monthlyResetAt"];
	        this.message = source["message"];
	    }
	}
	export class LiveAccountMetrics {
	    accountId: string;
	    status: string;
	    flow: LiveFlowQuota;
	    gemini: LiveGeminiQuota;
	    // Go type: time
	    retrievedAt: any;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new LiveAccountMetrics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accountId = source["accountId"];
	        this.status = source["status"];
	        this.flow = this.convertValues(source["flow"], LiveFlowQuota);
	        this.gemini = this.convertValues(source["gemini"], LiveGeminiQuota);
	        this.retrievedAt = this.convertValues(source["retrievedAt"], null);
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
	export class ManualAccountInput {
	    email: string;
	    cookies: string;
	    snlm0eToken: string;
	    proxy: string;
	    tier: string;
	    credits: number;
	    service: string;
	
	    static createFrom(source: any = {}) {
	        return new ManualAccountInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.cookies = source["cookies"];
	        this.snlm0eToken = source["snlm0eToken"];
	        this.proxy = source["proxy"];
	        this.tier = source["tier"];
	        this.credits = source["credits"];
	        this.service = source["service"];
	    }
	}
	export class ProxyTestResult {
	    success: boolean;
	    latencyMs: number;
	    egressIP?: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.latencyMs = source["latencyMs"];
	        this.egressIP = source["egressIP"];
	        this.message = source["message"];
	    }
	}
	export class RestoreResult {
	    success: boolean;
	    accountsRestored: number;
	    profilesRestored: number;
	    errors?: string[];
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new RestoreResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.accountsRestored = source["accountsRestored"];
	        this.profilesRestored = source["profilesRestored"];
	        this.errors = source["errors"];
	        this.message = source["message"];
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

