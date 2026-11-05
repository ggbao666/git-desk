export namespace main {
	
	export class GitActionResult {
	    ok: boolean;
	    root: string;
	    code: number;
	    conflicted: boolean;
	    output: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new GitActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.root = source["root"];
	        this.code = source["code"];
	        this.conflicted = source["conflicted"];
	        this.output = source["output"];
	        this.error = source["error"];
	    }
	}
	export class GitBranch {
	    name: string;
	    current: boolean;
	    upstream: string;
	
	    static createFrom(source: any = {}) {
	        return new GitBranch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.current = source["current"];
	        this.upstream = source["upstream"];
	    }
	}
	export class GitBranchDeleteResult {
	    ok: boolean;
	    root: string;
	    output: string;
	    notMerged: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new GitBranchDeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.root = source["root"];
	        this.output = source["output"];
	        this.notMerged = source["notMerged"];
	        this.error = source["error"];
	    }
	}
	export class GitBranchResult {
	    ok: boolean;
	    root: string;
	    branch: string;
	    output: string;
	    unchanged: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new GitBranchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.root = source["root"];
	        this.branch = source["branch"];
	        this.output = source["output"];
	        this.unchanged = source["unchanged"];
	        this.error = source["error"];
	    }
	}
	export class GitBranchesResult {
	    ok: boolean;
	    root: string;
	    branch: string;
	    branches: GitBranch[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new GitBranchesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.root = source["root"];
	        this.branch = source["branch"];
	        this.branches = this.convertValues(source["branches"], GitBranch);
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
	export class GitCommit {
	    hash: string;
	    shortHash: string;
	    author: string;
	    date: string;
	    parents: string[];
	    refs: string[];
	    subject: string;
	
	    static createFrom(source: any = {}) {
	        return new GitCommit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.shortHash = source["shortHash"];
	        this.author = source["author"];
	        this.date = source["date"];
	        this.parents = source["parents"];
	        this.refs = source["refs"];
	        this.subject = source["subject"];
	    }
	}
	export class GitCommitResult {
	    ok: boolean;
	    committed: boolean;
	    pushed: boolean;
	    root: string;
	    output: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new GitCommitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.committed = source["committed"];
	        this.pushed = source["pushed"];
	        this.root = source["root"];
	        this.output = source["output"];
	        this.error = source["error"];
	    }
	}
	export class GitDetectResult {
	    ok: boolean;
	    path: string;
	    version: string;
	    source: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new GitDetectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.source = source["source"];
	        this.error = source["error"];
	    }
	}
	export class GitFile {
	    path: string;
	    original: string;
	    index: string;
	    worktree: string;
	    kind: string;
	    conflicted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.original = source["original"];
	        this.index = source["index"];
	        this.worktree = source["worktree"];
	        this.kind = source["kind"];
	        this.conflicted = source["conflicted"];
	    }
	}
	export class GitHistoryResult {
	    ok: boolean;
	    root: string;
	    commits: GitCommit[];
	    hasMore: boolean;
	    empty: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new GitHistoryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.root = source["root"];
	        this.commits = this.convertValues(source["commits"], GitCommit);
	        this.hasMore = source["hasMore"];
	        this.empty = source["empty"];
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
	export class GitStatus {
	    ok: boolean;
	    isRepo: boolean;
	    root: string;
	    workspace: string;
	    branch: string;
	    remote: string;
	    upstream: string;
	    ahead: number;
	    behind: number;
	    changes: number;
	    conflicted: number;
	    mergeInProgress: boolean;
	    files: GitFile[];
	    gitPath: string;
	    gitVersion: string;
	    gitSource: string;
	    fetchError: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new GitStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.isRepo = source["isRepo"];
	        this.root = source["root"];
	        this.workspace = source["workspace"];
	        this.branch = source["branch"];
	        this.remote = source["remote"];
	        this.upstream = source["upstream"];
	        this.ahead = source["ahead"];
	        this.behind = source["behind"];
	        this.changes = source["changes"];
	        this.conflicted = source["conflicted"];
	        this.mergeInProgress = source["mergeInProgress"];
	        this.files = this.convertValues(source["files"], GitFile);
	        this.gitPath = source["gitPath"];
	        this.gitVersion = source["gitVersion"];
	        this.gitSource = source["gitSource"];
	        this.fetchError = source["fetchError"];
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
	export class IgnoreEntry {
	    name: string;
	    isDir: boolean;
	    ignored: boolean;
	
	    static createFrom(source: any = {}) {
	        return new IgnoreEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.isDir = source["isDir"];
	        this.ignored = source["ignored"];
	    }
	}
	export class IgnoreOverview {
	    dir: string;
	    exists: boolean;
	    entries: IgnoreEntry[];
	
	    static createFrom(source: any = {}) {
	        return new IgnoreOverview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.exists = source["exists"];
	        this.entries = this.convertValues(source["entries"], IgnoreEntry);
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
	export class IgnoreSaveResult {
	    ok: boolean;
	    created: boolean;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new IgnoreSaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.created = source["created"];
	        this.content = source["content"];
	    }
	}
	export class RecentItem {
	    path: string;
	    name: string;
	    openedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new RecentItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.openedAt = source["openedAt"];
	    }
	}
	export class Settings {
	    gitPath: string;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gitPath = source["gitPath"];
	        this.theme = source["theme"];
	    }
	}

}

