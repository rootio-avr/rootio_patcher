package io.root.patcher;

import org.gradle.api.GradleException;
import org.gradle.api.Plugin;
import org.gradle.api.Project;
import org.gradle.api.artifacts.ModuleVersionSelector;
import org.gradle.api.logging.Logger;

import java.util.Map;

public class RootIoPatcherPlugin implements Plugin<Project> {

    @Override
    public void apply(Project project) {
        // Fail fast if configuration cache is enabled — this plugin performs network I/O
        // during dependency resolution, which is incompatible with the configuration cache.
        // isConfigurationCacheRequested() is available since Gradle 7.0.
        if (project.getGradle().getStartParameter().isConfigurationCacheRequested()) {
            throw new GradleException(
                "io.root.patcher is not compatible with the Gradle configuration cache " +
                "(performs network I/O during dependency resolution).");
        }

        RootIoExtension ext = project.getExtensions().create("rootio", RootIoExtension.class);
        ext.getApiUrl().convention(envOrDefault("ROOTIO_API_URL", "https://api.root.io"));
        ext.getPkgUrl().convention(envOrDefault("ROOTIO_PKG_URL", "https://pkg.root.io"));
        ext.getTtlHours().convention(24L);
        ext.getVerbose().convention(false);
        // apiKey has no hardcoded default — it must come from env or build script
        String envApiKey = System.getenv("ROOTIO_API_KEY");
        if (envApiKey != null && !envApiKey.isEmpty()) {
            ext.getApiKey().convention(envApiKey);
        }

        // Auto-register the Root.io patches Maven repository so patched artifacts resolve
        // without users needing to add it manually. Done in afterEvaluate so apiKey/pkgUrl
        // are fully configured by the time we read them.
        project.afterEvaluate(p -> {
            String pkgBase = ext.getPkgUrl().get().replaceAll("/$", "");
            p.getRepositories().maven(repo -> {
                repo.setName("Root.io patches");
                repo.setUrl(pkgBase + "/maven");
                // Credentials only apply to HTTP(S) — file:// repos (e.g. in tests) reject them.
                if (pkgBase.startsWith("http://") || pkgBase.startsWith("https://")) {
                    repo.credentials(creds -> {
                        creds.setUsername(ext.getApiKey().get());
                        creds.setPassword("");
                    });
                }
            });
        });

        Logger logger = project.getLogger();

        project.getConfigurations().all(config -> {
            // Only hook resolvable configurations — non-resolvable ones (e.g. `api`, `implementation`)
            // are for declaring dependencies and cannot have eachDependency applied safely.
            if (!config.isCanBeResolved()) return;

            config.getResolutionStrategy().eachDependency(details -> {
                ModuleVersionSelector req = details.getRequested();
                String version = req.getVersion();

                // Skip deps with no version — these are BOM/platform-managed or Kotlin-plugin-managed
                // deps whose version is resolved separately. Sending an empty version to the API
                // produces a 400.
                if (version == null || version.isEmpty()) {
                    if (ext.getVerbose().get()) {
                        logger.lifecycle("[Root.io] Skipping {}:{} (no version)", req.getGroup(), req.getName());
                    }
                    return;
                }

                String coords = req.getGroup() + ":" + req.getName() + ":" + version;

                String patched = DepCache.lookup(
                    coords,
                    project.getRootDir(), // always rootDir so subprojects share the cache
                    ext.getTtlHours().get(),
                    () -> RootIoClient.query(coords, ext.getApiUrl().get(), ext.getApiKey().get())
                );

                if (patched != null) {
                    // Split "group:artifact:version" for useTarget map
                    int firstColon = patched.indexOf(':');
                    int lastColon  = patched.lastIndexOf(':');
                    String pGroup   = patched.substring(0, firstColon);
                    String pName    = patched.substring(firstColon + 1, lastColon);
                    String pVersion = patched.substring(lastColon + 1);
                    details.useTarget(Map.of("group", pGroup, "name", pName, "version", pVersion));
                    details.because("Root.io security patch");
                    if (ext.getVerbose().get()) {
                        logger.lifecycle("[Root.io] Patching {} -> {}", coords, patched);
                    }
                } else if (ext.getVerbose().get()) {
                    logger.lifecycle("[Root.io] No patch for {}", coords);
                }
            });
        });
    }

    private static String envOrDefault(String name, String defaultValue) {
        String val = System.getenv(name);
        return (val != null && !val.isEmpty()) ? val : defaultValue;
    }
}
