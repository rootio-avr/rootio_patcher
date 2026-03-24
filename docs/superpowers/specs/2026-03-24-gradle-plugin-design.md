# Design Doc: Gradle Plugin (`io.root.patcher`)

## Context

Root.io fixes CVE vulnerabilities in Java libraries by rebuilding them under a renamed Maven `groupId` (`io.root.<original>`). The `rootio_patcher` CLI already remediates Maven (`pom.xml`), pip, and npm projects. This document specifies a Gradle plugin that extends that capability to Gradle-based projects, implementing **Alternative 6** from the design exploration: an always-on `Plugin<Project>` that hooks `resolutionStrategy.eachDependency` with a per-dep local file cache.

---

## Goals

- Intercept every dependency (direct and transitive) during Gradle resolution and substitute patched Root.io coordinates when available
- No pipeline changes required — the plugin runs as part of every `./gradlew build`
- Full transitive coverage — `eachDependency` fires for every dep at every depth
- Low overhead on warm builds — per-dep file cache means most lookups are instant
- Fail the build on Root.io API errors — no silent misses
- Gradle 7.6+ compatible

---

## Non-Goals

- `Plugin<Settings>` (multi-module auto-registration) — deferred to a future version
- CLI-based init script generation (Alternative 1) — out of scope
- New dedicated per-dep API endpoint — reuses existing `POST /v3/analyze/maven`
- CI cache persistence strategy — left to the consumer

---

## Plugin Location

`gradle-plugin/` subdirectory of the `rootio_patcher` repository. A standalone Gradle/Java project, built and distributed independently from the Go CLI.

---

## Architecture

### Components

```
gradle-plugin/
├── build.gradle.kts
├── settings.gradle.kts
└── src/
    ├── main/java/io/root/patcher/
    │   ├── RootIoPatcherPlugin.java   # Plugin entry point
    │   ├── RootIoExtension.java       # Consumer DSL
    │   ├── DepCache.java              # File-based per-dep cache
    │   └── RootIoClient.java          # HTTP client (HttpURLConnection)
    └── test/java/io/root/patcher/
        ├── DepCacheTest.java
        ├── RootIoClientTest.java
        └── RootIoPatcherPluginFunctionalTest.java
```

### Data Flow

```
./gradlew build
  → configurations resolve (only resolvable configurations)
    → eachDependency fires for each dep (direct + transitive)
      → DepCache.lookup(coords, rootDir, ttlHours, onMiss)
          cache hit (file mtime < TTL)
            → return cached result (null = no patch, string = patched GAV)
          cache miss
            → RootIoClient.query(coords, apiUrl, apiKey)
                non-200 or network error → throw GradleException (build fails)
                no patch in response     → cache null, return null
                patch found              → cache "io.root.group:artifact:version"
                                           return patched GAV string
      → if patched != null: useTarget(map of group/name/version); because("Root.io security patch")
```

---

## Component Specifications

### `RootIoExtension`

Consumer-facing DSL block. All properties have defaults. Uses Gradle's managed properties pattern (abstract class with abstract getters).

```java
public abstract class RootIoExtension {
    public abstract Property<String> getApiKey();
    public abstract Property<String> getApiUrl();    // default: "https://api.root.io"
    public abstract Property<Long>   getTtlHours();  // default: 24
}
```

### `RootIoPatcherPlugin`

Implements `Plugin<Project>`. Registers the extension and hooks resolvable configurations at apply-time. The `eachDependency` callback fires later during resolution, by which time extension properties are set.

Key points:
- Only hooks configurations where `config.isCanBeResolved()` is true — avoids touching internal, non-resolvable, or consumer configurations, preventing `IllegalStateException` and redundant calls
- Checks `project.getGradle().getStartParameter().isConfigurationCacheRequested()` at apply-time and throws `GradleException` with a clear message if configuration cache is enabled — `notCompatibleWithConfigurationCache(String)` does not exist on `StartParameter` in Gradle 7.6; `isConfigurationCacheRequested()` is the correct stable API for a plugin-level guard
- Passes `project.getRootDir()` (not `project.getProjectDir()`) to `DepCache` so that all subprojects in a multi-module build share a single cache directory

```java
public class RootIoPatcherPlugin implements Plugin<Project> {
    @Override
    public void apply(Project project) {
        // Fail fast if configuration cache is enabled (not compatible)
        if (project.getGradle().getStartParameter().isConfigurationCacheRequested()) {
            throw new GradleException(
                "io.root.patcher is not compatible with the Gradle configuration cache " +
                "(performs network I/O during dependency resolution).");
        }

        RootIoExtension ext = project.getExtensions()
            .create("rootio", RootIoExtension.class);
        ext.getApiUrl().convention("https://api.root.io");
        ext.getTtlHours().convention(24L);

        project.getConfigurations().all(config -> {
            if (!config.isCanBeResolved()) return;
            config.getResolutionStrategy().eachDependency(details -> {
                ModuleVersionSelector req = details.getRequested();
                String coords = req.getGroup() + ":" + req.getName() + ":" + req.getVersion();
                String patched = DepCache.lookup(
                    coords,
                    project.getRootDir(),
                    ext.getTtlHours().get(),
                    () -> RootIoClient.query(coords, ext.getApiUrl().get(), ext.getApiKey().get())
                );
                if (patched != null) {
                    // Split "group:artifact:version" into parts for useTarget map
                    int lastColon = patched.lastIndexOf(':');
                    int firstColon = patched.indexOf(':');
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
```

### `DepCache`

File-based cache. One JSON file per unique dep coordinate.

- **Cache dir:** `<rootDir>/.gradle/rootio-cache/` — always `rootDir` so subprojects share one cache
- **File name:** SHA-1 hex of the coordinate string (e.g. `a3f2c1...json`) — avoids any filename collision and handles special characters in coordinates safely
- **File content:** `{"patched":"io.root.org.eclipse.jetty:jetty-http:11.0.26"}` or `{"patched":null}` — written with minimal string construction, no JSON library needed
- **TTL check:** `System.currentTimeMillis() - file.lastModified() < ttlHours * 3_600_000L`
- **Interface:**
  ```java
  public static String lookup(String coords, File rootDir, long ttlHours, Supplier<String> onMiss)
  ```
  Returns cached value if file exists and is within TTL; otherwise calls `onMiss`, writes result to file, and returns result.

**Note on parallel builds (`--parallel`):** Two subprojects resolving the same dep simultaneously could both get a cache miss before either writes the result, causing two API calls for that dep. Acceptable for this version — the result is idempotent and the race is rare. File locking can be added later if needed.

### `RootIoClient`

HTTP client with no external dependencies (uses `java.net.HttpURLConnection`). No JSON library — uses minimal string operations.

- **Interface:**
  ```java
  public static String query(String coords, String apiUrl, String apiKey)
  ```
- **Request:** `POST {apiUrl}/v3/analyze/maven` with basic auth (`apiKey` as username, empty password)
  ```json
  {"packages": [{"name": "group:artifact", "version": "x.y.z"}]}
  ```
  `coords` `"group:artifact:version"` is split on colons: first segment is `group`, last segment is `version`, middle segment(s) are `artifact`.

- **`patch_alias` field contract:** The Root.io API response `patches[0].patch_alias.name` is a full `groupId:artifactId` string (e.g. `"io.root.org.eclipse.jetty:jetty-http"`), and `patches[0].patch_alias.version` is the version string. The client concatenates them as `name + ":" + version` to produce a three-segment `group:artifact:version` GAV string (e.g. `"io.root.org.eclipse.jetty:jetty-http:11.0.26"`). This format is what is stored in the cache and passed to `useTarget`.

- **Response parsing:** Implemented via `String.indexOf` / `substring` — no external JSON library. Extracts `patch_alias.name` and `patch_alias.version` from the response body. Returns the combined GAV string if present, `null` if `patches` array is empty.

- **Error handling:** throws `GradleException` on non-200 HTTP status or any `IOException`.

### `build.gradle.kts`

`java-gradle-plugin` configures publications when `maven-publish` is present but does not auto-apply it. `maven-publish` must be declared explicitly to make `publishToMavenLocal` available.

```kotlin
plugins {
    `java-gradle-plugin`
    `maven-publish`
}

group = "io.root"
version = "0.1.0"

java {
    sourceCompatibility = JavaVersion.VERSION_11
    targetCompatibility = JavaVersion.VERSION_11
}

gradlePlugin {
    plugins {
        create("rootIoPatcher") {
            id = "io.root.patcher"
            implementationClass = "io.root.patcher.RootIoPatcherPlugin"
        }
    }
}

dependencies {
    testImplementation(gradleTestKit())
    testImplementation("org.junit.jupiter:junit-jupiter:5.10.0")
}

tasks.test {
    useJUnitPlatform()
}
```

---

## Consumer Usage

### Single-module project

Note: `Property<T>` requires `.set()` in Kotlin DSL. `Long` literals require the `L` suffix.

```kotlin
// build.gradle.kts
plugins {
    id("io.root.patcher") version "0.1.0"
}

rootio {
    apiKey.set(System.getenv("ROOTIO_API_KEY"))
    // apiUrl.set("https://api.root.io")  // optional, this is the default
    // ttlHours.set(24L)                  // optional, this is the default
}
```

### Multi-module project

```kotlin
// root build.gradle.kts
subprojects {
    apply(plugin = "io.root.patcher")
    extensions.configure<io.root.patcher.RootIoExtension> {
        apiKey.set(System.getenv("ROOTIO_API_KEY"))
    }
}
```

Add to `.gitignore`:
```
.gradle/rootio-cache/
```

---

## Testing Strategy

### Unit tests (JUnit 5, Java)

**`DepCacheTest`:**
- Cache miss calls `onMiss` and writes result to file
- Cache hit returns cached value without calling `onMiss`
- Expired TTL (mtime older than `ttlHours`) calls `onMiss` again
- `null` (no patch) is cached and returned correctly on next lookup without re-querying

**`RootIoClientTest`:**

Uses `com.sun.net.httpserver.HttpServer` as a local mock server. This is a JDK internal API but has been stable since Java 6 and is acceptable for a POC. On Java 18+ it is a proper API (`jdk.httpserver` module).

- Parses a valid API response and returns patched GAV string
- Returns `null` when `patches` array is empty
- Throws `GradleException` on HTTP 401/500
- Throws `GradleException` on connection failure

### Functional tests (Gradle TestKit, Java)

**`RootIoPatcherPluginFunctionalTest`:**
- Creates a temp project with `build.gradle.kts` applying the plugin and a declared dependency
- Starts a local `HttpServer` mock for the Root.io API
- **Case 1:** API returns a patch → `./gradlew dependencies` output contains `-> io.root.*` with reason `Root.io security patch`
- **Case 2:** API returns empty patches → original dep is unchanged
- **Case 3:** API returns 500 → build fails with a `GradleException`

---

## Testing Against Real Open-Source Projects

Once the plugin is published locally (via `./gradlew publishToMavenLocal`), you can test it against real Gradle projects.

### Setup

```bash
# 1. Build and publish to local Maven repository
cd gradle-plugin
./gradlew publishToMavenLocal

# 2. Set your API key
export ROOTIO_API_KEY=<your-key>
```

### Applying the plugin to a target project

In the target project's `settings.gradle.kts`, add the local Maven repo to plugin resolution:

```kotlin
pluginManagement {
    repositories {
        mavenLocal()
        gradlePluginPortal()
    }
}
```

Then in `build.gradle.kts` (or root `build.gradle.kts` for multi-module):

```kotlin
plugins {
    id("io.root.patcher") version "0.1.0"
}
rootio {
    apiKey.set(System.getenv("ROOTIO_API_KEY"))
}
```

### Suggested open-source projects to test against

These are well-known Gradle projects with rich transitive dependency trees:

| Project | Why useful |
|---|---|
| [Spring PetClinic REST](https://github.com/spring-petclinic/spring-petclinic-rest) | Spring Boot + many transitive deps, single module |
| [Micronaut Starter](https://github.com/micronaut-projects/micronaut-starter) | Multi-module, varied dep graph |
| [OpenTelemetry Java](https://github.com/open-telemetry/opentelemetry-java) | Large multi-module project |
| [Quarkus](https://github.com/quarkusio/quarkus) | Massive dep graph, good stress test for cold cache overhead |

### What to verify

1. **Cold cache:** Run `./gradlew dependencies` — observe API calls being made, confirm `.gradle/rootio-cache/` is populated
2. **Warm cache:** Run again immediately — near-zero overhead, no API calls
3. **Substitutions visible:** Run `./gradlew dependencies | grep "io.root\."` — substituted deps appear as `original -> io.root.*` with reason `Root.io security patch`
4. **Build succeeds:** Run `./gradlew build` end-to-end with the plugin applied
5. **Cache wipe:** Delete `.gradle/rootio-cache/` and re-run to simulate cold cache again

---

## Open Questions

1. **Plugin Portal distribution:** When ready for customers, the plugin needs to be published to the Gradle Plugin Portal or a self-hosted Maven repo. Not in scope for the POC.
2. **`--parallel` race condition:** Acceptable for now. Can add file locking if it becomes a real issue.
3. **Batch API optimization:** Currently one API call per cache miss. If cold-cache performance on large projects is unacceptable, a bulk batch (collect all cache misses, one API call) can be added — but requires architectural change.
4. **CI cache persistence:** Ephemeral CI agents (e.g. GitHub Actions) start cold on every run. Consumers should use `actions/cache` to persist `.gradle/rootio-cache/`. Instructions to be added to the consumer guide when the plugin is production-ready.