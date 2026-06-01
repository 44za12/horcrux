export namespace main {
	
	export class AddProviderRequest {
	    ProviderType: string;
	    Name: string;
	    ClientID?: string;
	    ClientSecret?: string;
	    Endpoint?: string;
	    Region?: string;
	    Bucket?: string;
	    AccessKey?: string;
	    SecretKey?: string;
	    Path?: string;
	    Host?: string;
	    Port?: string;
	    Username?: string;
	    Password?: string;
	    KeyPath?: string;
	    RemotePath?: string;
	
	    static createFrom(source: any = {}) {
	        return new AddProviderRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ProviderType = source["ProviderType"];
	        this.Name = source["Name"];
	        this.ClientID = source["ClientID"];
	        this.ClientSecret = source["ClientSecret"];
	        this.Endpoint = source["Endpoint"];
	        this.Region = source["Region"];
	        this.Bucket = source["Bucket"];
	        this.AccessKey = source["AccessKey"];
	        this.SecretKey = source["SecretKey"];
	        this.Path = source["Path"];
	        this.Host = source["Host"];
	        this.Port = source["Port"];
	        this.Username = source["Username"];
	        this.Password = source["Password"];
	        this.KeyPath = source["KeyPath"];
	        this.RemotePath = source["RemotePath"];
	    }
	}
	export class DistributeStatus {
	    Total: number;
	    Threshold: number;
	    CanDistribute: boolean;
	    CanRestore: boolean;
	    Failures: number;
	
	    static createFrom(source: any = {}) {
	        return new DistributeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Total = source["Total"];
	        this.Threshold = source["Threshold"];
	        this.CanDistribute = source["CanDistribute"];
	        this.CanRestore = source["CanRestore"];
	        this.Failures = source["Failures"];
	    }
	}
	export class ProviderInfo {
	    Name: string;
	    Type: string;
	    Status: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Type = source["Type"];
	        this.Status = source["Status"];
	    }
	}
	export class ProviderTypes {
	    Id: string;
	    Name: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderTypes(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Id = source["Id"];
	        this.Name = source["Name"];
	    }
	}

}

export namespace vault {
	
	export class ApiKeyEntry {
	    Service: string;
	    Name: string;
	    Key: string;
	    Notes: string;
	
	    static createFrom(source: any = {}) {
	        return new ApiKeyEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Service = source["Service"];
	        this.Name = source["Name"];
	        this.Key = source["Key"];
	        this.Notes = source["Notes"];
	    }
	}
	export class FileEntry {
	    Name: string;
	    MimeType: string;
	    Size: number;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.MimeType = source["MimeType"];
	        this.Size = source["Size"];
	    }
	}
	export class PasswordEntry {
	    Site: string;
	    Username: string;
	    Password: string;
	    Notes: string;
	
	    static createFrom(source: any = {}) {
	        return new PasswordEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Site = source["Site"];
	        this.Username = source["Username"];
	        this.Password = source["Password"];
	        this.Notes = source["Notes"];
	    }
	}
	export class TotpService {
	    Name: string;
	    Secret: string;
	
	    static createFrom(source: any = {}) {
	        return new TotpService(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Secret = source["Secret"];
	    }
	}

}

