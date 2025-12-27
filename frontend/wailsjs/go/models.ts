export namespace proxy {
	
	export class EncodingOption {
	    value: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new EncodingOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class HTMLVersionOption {
	    value: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new HTMLVersionOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class ImageFormatOption {
	    value: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new ImageFormatOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}

}

