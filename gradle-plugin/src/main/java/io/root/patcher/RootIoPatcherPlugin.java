package io.root.patcher;

import org.gradle.api.GradleException;
import org.gradle.api.Plugin;
import org.gradle.api.Project;
import org.gradle.api.artifacts.ModuleVersionSelector;

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
        ext.getApiUrl().convention("https://api.root.io");
        ext.getTtlHours().convention(24L);

        project.getConfigurations().all(config -> {
            // Only hook resolvable configurations — non-resolvable ones (e.g. `api`, `implementation`)
            // are for declaring dependencies and cannot have eachDependency applied safely.
            if (!config.isCanBeResolved()) return;

            config.getResolutionStrategy().eachDependency(details -> {
                ModuleVersionSelector req = details.getRequested();
                String coords = req.getGroup() + ":" + req.getName() + ":" + req.getVersion();

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
                    String group   = patched.substring(0, firstColon);
                    String name    = patched.substring(firstColon + 1, lastColon);
                    String version = patched.substring(lastColon + 1);
                    details.useTarget(Map.of("group", group, "name", name, "version", version));
                    details.because("Root.io security patch");
                }
            });
        });
    }
}
