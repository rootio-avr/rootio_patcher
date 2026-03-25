package io.root.patcher;

import org.gradle.api.provider.Property;

public abstract class RootIoExtension {
    /** Root.io API key (required). Set via rootio { apiKey.set(...) }. */
    public abstract Property<String> getApiKey();

    /** Root.io API base URL. Default: https://api.root.io */
    public abstract Property<String> getApiUrl();

    /** Cache TTL in hours. Default: 24. Set to 0 for no caching. */
    public abstract Property<Long> getTtlHours();

    /** Enable verbose lifecycle logging. Default: false. */
    public abstract Property<Boolean> getVerbose();

    /** Root.io package registry base URL. Default: https://pkg.root.io */
    public abstract Property<String> getPkgUrl();
}
