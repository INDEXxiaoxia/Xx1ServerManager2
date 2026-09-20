export namespace models {
	
	export class CommandHistory {
	    id: number;
	    serverId: string;
	    serverName: string;
	    command: string;
	    status: string;
	    exitCode: number;
	    stdout: string;
	    stderr: string;
	    duration: number;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new CommandHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.serverId = source["serverId"];
	        this.serverName = source["serverName"];
	        this.command = source["command"];
	        this.status = source["status"];
	        this.exitCode = source["exitCode"];
	        this.stdout = source["stdout"];
	        this.stderr = source["stderr"];
	        this.duration = source["duration"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	export class DirSizeItem {
	    path: string;
	    size: number;
	    method: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new DirSizeItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.size = source["size"];
	        this.method = source["method"];
	        this.error = source["error"];
	    }
	}
	export class FileEntry {
	    name: string;
	    path: string;
	    isDir: boolean;
	    size: number;
	    mode: string;
	    modTime: string;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.mode = source["mode"];
	        this.modTime = source["modTime"];
	    }
	}
	export class Group {
	    id: string;
	    name: string;
	    description: string;
	    color: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.color = source["color"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	export class MonitorPoint {
	    id: number;
	    serverId: string;
	    cpuUsage: number;
	    memTotal: number;
	    memUsed: number;
	    memUsage: number;
	    diskTotal: number;
	    diskUsed: number;
	    diskUsage: number;
	    netRx: number;
	    netTx: number;
	    // Go type: time
	    timestamp: any;
	
	    static createFrom(source: any = {}) {
	        return new MonitorPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.serverId = source["serverId"];
	        this.cpuUsage = source["cpuUsage"];
	        this.memTotal = source["memTotal"];
	        this.memUsed = source["memUsed"];
	        this.memUsage = source["memUsage"];
	        this.diskTotal = source["diskTotal"];
	        this.diskUsed = source["diskUsed"];
	        this.diskUsage = source["diskUsage"];
	        this.netRx = source["netRx"];
	        this.netTx = source["netTx"];
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
	export class PresetCommand {
	    id: string;
	    name: string;
	    command: string;
	    description: string;
	    category: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new PresetCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.command = source["command"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	export class PresetPath {
	    id: string;
	    name: string;
	    path: string;
	    description: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new PresetPath(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.description = source["description"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	export class SearchResult {
	    root: string;
	    keyword: string;
	    recursive: boolean;
	    items: FileEntry[];
	    truncated: boolean;
	    scanned: number;
	    elapsedMs: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.keyword = source["keyword"];
	        this.recursive = source["recursive"];
	        this.items = this.convertValues(source["items"], FileEntry);
	        this.truncated = source["truncated"];
	        this.scanned = source["scanned"];
	        this.elapsedMs = source["elapsedMs"];
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
	export class Server {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    password?: string;
	    groupIds: string[];
	    status: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.groupIds = source["groupIds"];
	        this.status = source["status"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class Settings {
	    maxConcurrent: number;
	    uploadTimeout: number;
	    commandTimeout: number;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxConcurrent = source["maxConcurrent"];
	        this.uploadTimeout = source["uploadTimeout"];
	        this.commandTimeout = source["commandTimeout"];
	        this.theme = source["theme"];
	    }
	}
	export class SystemService {
	    name: string;
	    description: string;
	    loadState: string;
	    activeState: string;
	    subState: string;
	    enabled: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemService(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.loadState = source["loadState"];
	        this.activeState = source["activeState"];
	        this.subState = source["subState"];
	        this.enabled = source["enabled"];
	    }
	}
	export class TextFileContent {
	    path: string;
	    size: number;
	    base64: string;
	    encoding: string;
	    hasBOM: boolean;
	    eol: string;
	    eolMixed: boolean;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TextFileContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.size = source["size"];
	        this.base64 = source["base64"];
	        this.encoding = source["encoding"];
	        this.hasBOM = source["hasBOM"];
	        this.eol = source["eol"];
	        this.eolMixed = source["eolMixed"];
	        this.truncated = source["truncated"];
	    }
	}
	export class TextFileMeta {
	    path: string;
	    name: string;
	    size: number;
	    encoding: string;
	    hasBOM: boolean;
	    binary: boolean;
	    eol: string;
	    eolMixed: boolean;
	    editable: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new TextFileMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.encoding = source["encoding"];
	        this.hasBOM = source["hasBOM"];
	        this.binary = source["binary"];
	        this.eol = source["eol"];
	        this.eolMixed = source["eolMixed"];
	        this.editable = source["editable"];
	        this.message = source["message"];
	    }
	}
	export class TextFileWriteRequest {
	    text: string;
	    encoding: string;
	    keepBOM: boolean;
	    eol: string;
	
	    static createFrom(source: any = {}) {
	        return new TextFileWriteRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.encoding = source["encoding"];
	        this.keepBOM = source["keepBOM"];
	        this.eol = source["eol"];
	    }
	}

}

