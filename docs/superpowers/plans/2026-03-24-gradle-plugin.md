# Gradle Plugin (`io.root.patcher`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Gradle `Plugin<Project>` (Java) that hooks `resolutionStrategy.eachDependency` to intercept all dependencies (direct and transitive) and substitute patched Root.io coordinates, using a per-dep file cache with a 24h TTL.

**Architecture:** A standalone Gradle/Java project in `gradle-plugin/`. Four classes: `RootIoExtension` (consumer DSL), `DepCache` (file-based TTL cache), `RootIoClient` (HTTP calls via `HttpURLConnection`), and `RootIoPatcherPlugin` (wires them together). TDD throughout: tests are written before implementation for every component with logic.

**Tech Stack:** Java 11, Gradle 7.6+, JUnit 5, Gradle TestKit, `java.net.HttpURLConnection`, `com.sun.net.httpserver.HttpServer` (mock in tests), `java.security.MessageDigest` (SHA-1 for cache filenames).

---

## File Map

| File | Responsibility |
|---|---|
| `gradle-plugin/settings.gradle.kts` | Plugin project name |
| `gradle-plugin/build.gradle.kts` | Build config, plugin registration, dependencies |
| `gradle-plugin/src/main/java/io/root/patcher/RootIoExtension.java` | Abstract Gradle extension (apiKey, apiUrl, ttlHours) |
| `gradle-plugin/src/main/java/io/root/patcher/DepCache.java` | File-based per-dep cache with TTL; SHA-1 filenames |
| `gradle-plugin/src/main/java/io/root/patcher/RootIoClient.java` | HTTP POST to `/v3/analyze/maven`; manual JSON parsing |
| `gradle-plugin/src/main/java/io/root/patcher/RootIoPatcherPlugin.java` | Plugin entry point; registers extension; hooks eachDependency |
| `gradle-plugin/src/test/java/io/root/patcher/DepCacheTest.java` | Unit tests for cache logic |
| `gradle-plugin/src/test/java/io/root/patcher/RootIoClientTest.java` | Unit tests for HTTP client with local HttpServer mock |
| `gradle-plugin/src/test/java/io/root/patcher/RootIoPatcherPluginFunctionalTest.java` | Gradle TestKit functional tests |

---

## Task 1: Scaffold the project

**Files:**
- Create: `gradle-plugin/settings.gradle.kts`
- Create: `gradle-plugin/build.gradle.kts`
- Modify: `.gitignore`

- [ ] **Step 1: Verify Gradle is installed locally**

```bash
gradle --version
```
Expected: Gradle 7.6 or later. If not installed: `brew install gradle` (macOS).

- [ ] **Step 2: Create the `gradle-plugin/` directory and generate the wrapper**

```bash
mkdir -p gradle-plugin && cd gradle-plugin && gradle wrapper --gradle-version 8.5
```

Expected output: `gradle-plugin/gradlew`, `gradle-plugin/gradlew.bat`, `gradle-plugin/gradle/wrapper/gradle-wrapper.jar`, `gradle-plugin/gradle/wrapper/gradle-wrapper.properties` are created.

- [ ] **Step 3: Create `gradle-plugin/settings.gradle.kts`**

```kotlin
rootProject.name = "rootio-patcher-gradle-plugin"
```

- [ ] **Step 4: Create `gradle-plugin/build.gradle.kts`**

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
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

tasks.test {
    useJUnitPlatform()
}
```

- [ ] **Step 5: Create source directories**

```bash
mkdir -p gradle-plugin/src/main/java/io/root/patcher
mkdir -p gradle-plugin/src/test/java/io/root/patcher
```

- [ ] **Step 6: Update `.gitignore` at the repo root**

Append to the existing `/Users/idoberko2/root/rootio_patcher/.gitignore` (create it if absent):

```
# Gradle plugin
gradle-plugin/.gradle/
gradle-plugin/build/
gradle-plugin/.gradle/rootio-cache/
```

- [ ] **Step 7: Verify the project compiles (no source yet is fine)**

```bash
cd gradle-plugin && ./gradlew help
```
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 8: Commit**

```bash
cd /Users/idoberko2/root/rootio_patcher
rtk git add gradle-plugin/ .gitignore
rtk git commit -m "feat: scaffold gradle-plugin project"
```

---

## Task 2: `RootIoExtension`

**Files:**
- Create: `gradle-plugin/src/main/java/io/root/patcher/RootIoExtension.java`

No unit test needed — it is a pure property container. The functional test in Task 5 exercises it end-to-end.

- [ ] **Step 1: Create `RootIoExtension.java`**

```java
package io.root.patcher;

import org.gradle.api.provider.Property;

public abstract class RootIoExtension {
    /** Root.io API key (required). Set via ROOTIO_API_KEY env var or rootio { apiKey.set(...) }. */
    public abstract Property<String> getApiKey();

    /** Root.io API base URL. Default: https://api.root.io */
    public abstract Property<String> getApiUrl();

    /** Cache TTL in hours. Default: 24. Set to 0 for no caching. */
    public abstract Property<Long> getTtlHours();
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd gradle-plugin && ./gradlew compileJava
```
Expected: `BUILD SUCCESSFUL`

- [ ] **Step 3: Commit**

```bash
cd /Users/idoberko2/root/rootio_patcher
rtk git add gradle-plugin/src/main/java/io/root/patcher/RootIoExtension.java
rtk git commit -m "feat: add RootIoExtension"
```

---

## Task 3: `DepCache` (TDD)

**Files:**
- Create: `gradle-plugin/src/test/java/io/root/patcher/DepCacheTest.java`
- Create: `gradle-plugin/src/main/java/io/root/patcher/DepCache.java`

- [ ] **Step 1: Write the failing tests**

Create `gradle-plugin/src/test/java/io/root/patcher/DepCacheTest.java`:

```java
package io.root.patcher;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.io.File;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;

class DepCacheTest {

    @TempDir
    File tempDir;

    @Test
    void cacheMissCallsOnMissAndWritesResult() {
        AtomicInteger callCount = new AtomicInteger(0);

        String result = DepCache.lookup("org.example:foo:1.0", tempDir, 24, () -> {
            callCount.incrementAndGet();
            return "io.root.org.example:foo:1.0-patched";
        });

        assertEquals("io.root.org.example:foo:1.0-patched", result);
        assertEquals(1, callCount.get());
    }

    @Test
    void cacheHitSkipsOnMiss() {
        // Warm the cache
        DepCache.lookup("org.example:bar:2.0", tempDir, 24, () -> "io.root.org.example:bar:2.0-patched");

        AtomicInteger callCount = new AtomicInteger(0);
        String result = DepCache.lookup("org.example:bar:2.0", tempDir, 24, () -> {
            callCount.incrementAndGet();
            return "should-not-be-called";
        });

        assertEquals("io.root.org.example:bar:2.0-patched", result);
        assertEquals(0, callCount.get());
    }

    @Test
    void expiredTtlCallsOnMissAgain() {
        // Warm the cache
        DepCache.lookup("org.example:baz:3.0", tempDir, 24, () -> "io.root.org.example:baz:3.0-patched");

        // Backdate the cache file past the TTL
        File cacheDir = new File(tempDir, ".gradle/rootio-cache");
        File[] files = cacheDir.listFiles();
        assertNotNull(files);
        assertEquals(1, files.length);
        assertTrue(files[0].setLastModified(System.currentTimeMillis() - 25 * 3_600_000L));

        AtomicInteger callCount = new AtomicInteger(0);
        DepCache.lookup("org.example:baz:3.0", tempDir, 24, () -> {
            callCount.incrementAndGet();
            return "refreshed";
        });

        assertEquals(1, callCount.get());
    }

    @Test
    void nullResultIsCachedAndNotRefetched() {
        // Cache a null (no patch for this dep)
        DepCache.lookup("org.example:qux:4.0", tempDir, 24, () -> null);

        AtomicInteger callCount = new AtomicInteger(0);
        String result = DepCache.lookup("org.example:qux:4.0", tempDir, 24, () -> {
            callCount.incrementAndGet();
            return "should-not-be-called";
        });

        assertNull(result);
        assertEquals(0, callCount.get());
    }
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

```bash
cd gradle-plugin && ./gradlew test --tests "io.root.patcher.DepCacheTest"
```
Expected: `FAILED` — `DepCache` does not exist yet.

- [ ] **Step 3: Implement `DepCache.java`**

Create `gradle-plugin/src/main/java/io/root/patcher/DepCache.java`:

```java
package io.root.patcher;

import java.io.File;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.function.Supplier;

public class DepCache {

    private static final String CACHE_SUBDIR = ".gradle/rootio-cache";

    /**
     * Look up the patched coordinates for {@code coords} from the local cache.
     * On a cache miss (file absent or older than {@code ttlHours}), calls {@code onMiss},
     * writes the result to the cache, and returns it.
     *
     * @param coords    Maven GAV string — "group:artifact:version"
     * @param rootDir   project root directory (always use {@code project.getRootDir()})
     * @param ttlHours  cache TTL in hours
     * @param onMiss    called when the cache is cold or stale; may return null (no patch)
     * @return patched GAV string, or null if no patch exists
     */
    public static String lookup(String coords, File rootDir, long ttlHours, Supplier<String> onMiss) {
        File cacheFile = cacheFile(coords, rootDir);
        if (cacheFile.exists() && isWithinTtl(cacheFile, ttlHours)) {
            return readCache(cacheFile);
        }
        String result = onMiss.get();
        writeCache(cacheFile, result);
        return result;
    }

    private static boolean isWithinTtl(File file, long ttlHours) {
        long ageMs = System.currentTimeMillis() - file.lastModified();
        return ageMs < ttlHours * 3_600_000L;
    }

    private static File cacheFile(String coords, File rootDir) {
        File dir = new File(rootDir, CACHE_SUBDIR);
        dir.mkdirs();
        return new File(dir, sha1(coords) + ".json");
    }

    private static String readCache(File file) {
        try {
            String content = Files.readString(file.toPath());
            if (content.contains("\"patched\":null")) return null;
            int start = content.indexOf("\"patched\":\"") + 11;
            int end = content.indexOf('"', start);
            if (start < 11 || end <= start) return null;
            return content.substring(start, end);
        } catch (IOException e) {
            return null; // treat read failure as cache miss
        }
    }

    private static void writeCache(File file, String patchedCoords) {
        String content = patchedCoords == null
            ? "{\"patched\":null}"
            : "{\"patched\":\"" + patchedCoords + "\"}";
        try {
            Files.writeString(file.toPath(), content);
        } catch (IOException e) {
            // swallow — cache write failure is non-fatal; the build continues
        }
    }

    private static String sha1(String input) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-1");
            byte[] hash = md.digest(input.getBytes(StandardCharsets.UTF_8));
            StringBuilder sb = new StringBuilder();
            for (byte b : hash) sb.append(String.format("%02x", b));
            return sb.toString();
        } catch (NoSuchAlgorithmException e) {
            throw new RuntimeException("SHA-1 not available", e);
        }
    }
}
```

- [ ] **Step 4: Run tests and confirm they pass**

```bash
cd gradle-plugin && ./gradlew test --tests "io.root.patcher.DepCacheTest"
```
Expected: `BUILD SUCCESSFUL`, 4 tests passed.

- [ ] **Step 5: Commit**

```bash
cd /Users/idoberko2/root/rootio_patcher
rtk git add gradle-plugin/src/
rtk git commit -m "feat: add DepCache with TTL and SHA-1 file keys"
```

---

## Task 4: `RootIoClient` (TDD)

**Files:**
- Create: `gradle-plugin/src/test/java/io/root/patcher/RootIoClientTest.java`
- Create: `gradle-plugin/src/main/java/io/root/patcher/RootIoClient.java`

- [ ] **Step 1: Write the failing tests**

Create `gradle-plugin/src/test/java/io/root/patcher/RootIoClientTest.java`:

```java
package io.root.patcher;

import com.sun.net.httpserver.HttpServer;
import org.gradle.api.GradleException;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.*;

class RootIoClientTest {

    private HttpServer server;
    private int port;

    @BeforeEach
    void startServer() throws IOException {
        server = HttpServer.create(new InetSocketAddress(0), 0);
        port = server.getAddress().getPort();
        server.start();
    }

    @AfterEach
    void stopServer() {
        server.stop(0);
    }

    @Test
    void returnsNullWhenNoPatchAvailable() {
        respondWith(200, "{\"patches\":[],\"skipped\":[]}");

        String result = RootIoClient.query("org.example:foo:1.0", "http://localhost:" + port, "test-key");

        assertNull(result);
    }

    @Test
    void returnsPatchedCoordsWhenPatchAvailable() {
        respondWith(200,
            "{\"patches\":[{" +
            "\"package_name\":\"org.example:foo\",\"version\":\"1.0\"," +
            "\"patch\":{\"name\":\"io.root.org.example:foo\",\"version\":\"1.0\"}," +
            "\"patch_alias\":{\"name\":\"io.root.org.example:foo\",\"version\":\"1.0-patched\"}," +
            "\"cve_ids\":[]}]," +
            "\"skipped\":[]}");

        String result = RootIoClient.query("org.example:foo:1.0", "http://localhost:" + port, "test-key");

        assertEquals("io.root.org.example:foo:1.0-patched", result);
    }

    @Test
    void throwsGradleExceptionOn500() {
        respondWith(500, "");

        assertThrows(GradleException.class, () ->
            RootIoClient.query("org.example:foo:1.0", "http://localhost:" + port, "test-key"));
    }

    @Test
    void throwsGradleExceptionOn401() {
        respondWith(401, "");

        assertThrows(GradleException.class, () ->
            RootIoClient.query("org.example:foo:1.0", "http://localhost:" + port, "test-key"));
    }

    @Test
    void throwsGradleExceptionOnConnectionFailure() {
        // Port 1 has no server — connection will be refused
        assertThrows(GradleException.class, () ->
            RootIoClient.query("org.example:foo:1.0", "http://localhost:1", "test-key"));
    }

    private void respondWith(int status, String body) {
        server.createContext("/v3/analyze/maven", exchange -> {
            byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
            exchange.sendResponseHeaders(status, status == 200 ? bytes.length : -1);
            if (status == 200) {
                try (OutputStream os = exchange.getResponseBody()) {
                    os.write(bytes);
                }
            } else {
                exchange.getResponseBody().close();
            }
        });
    }
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

```bash
cd gradle-plugin && ./gradlew test --tests "io.root.patcher.RootIoClientTest"
```
Expected: `FAILED` — `RootIoClient` does not exist yet.

- [ ] **Step 3: Implement `RootIoClient.java`**

Create `gradle-plugin/src/main/java/io/root/patcher/RootIoClient.java`:

```java
package io.root.patcher;

import org.gradle.api.GradleException;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.Base64;

public class RootIoClient {

    /**
     * Query the Root.io API for a patch for the given dependency.
     *
     * @param coords  Maven GAV string — "group:artifact:version"
     * @param apiUrl  Root.io API base URL (e.g. "https://api.root.io")
     * @param apiKey  Root.io API key (used as HTTP basic auth username)
     * @return patched GAV string ("io.root.group:artifact:version"), or null if no patch
     * @throws GradleException on non-200 response or network failure (fails the build)
     */
    public static String query(String coords, String apiUrl, String apiKey) {
        // Split "group:artifact:version" — last colon separates version
        int lastColon = coords.lastIndexOf(':');
        String groupArtifact = coords.substring(0, lastColon);
        String version = coords.substring(lastColon + 1);

        String requestBody = "{\"packages\":[{\"name\":\"" + groupArtifact + "\",\"version\":\"" + version + "\"}]}";
        String endpoint = apiUrl.replaceAll("/$", "") + "/v3/analyze/maven";

        try {
            HttpURLConnection conn = (HttpURLConnection) new URL(endpoint).openConnection();
            conn.setRequestMethod("POST");
            conn.setRequestProperty("Content-Type", "application/json");
            String credentials = Base64.getEncoder()
                .encodeToString((apiKey + ":").getBytes(StandardCharsets.UTF_8));
            conn.setRequestProperty("Authorization", "Basic " + credentials);
            conn.setDoOutput(true);

            try (OutputStream os = conn.getOutputStream()) {
                os.write(requestBody.getBytes(StandardCharsets.UTF_8));
            }

            int status = conn.getResponseCode();
            if (status != 200) {
                throw new GradleException(
                    "Root.io API returned HTTP " + status + " for " + coords);
            }

            try (InputStream is = conn.getInputStream()) {
                String response = new String(is.readAllBytes(), StandardCharsets.UTF_8);
                return extractPatchedCoords(response);
            }
        } catch (GradleException e) {
            throw e;
        } catch (IOException e) {
            throw new GradleException(
                "Root.io API request failed for " + coords + ": " + e.getMessage(), e);
        }
    }

    // Package-private for testability
    static String extractPatchedCoords(String json) {
        // Verify patches array is non-empty
        int patchesIdx = json.indexOf("\"patches\"");
        if (patchesIdx == -1) return null;
        int arrayStart = json.indexOf('[', patchesIdx);
        if (arrayStart == -1) return null;
        int i = arrayStart + 1;
        while (i < json.length() && Character.isWhitespace(json.charAt(i))) i++;
        if (i >= json.length() || json.charAt(i) == ']') return null; // empty array

        // Find "patch_alias" key and extract its object as a substring.
        // PatchInfo has only string fields (name, version) — no nested objects —
        // so indexOf('}') correctly finds the end of the patch_alias object.
        // Bounding to this substring prevents accidental matches against sibling
        // fields like "patch.version" or the outer "version" field.
        int aliasIdx = json.indexOf("\"patch_alias\"", patchesIdx);
        if (aliasIdx == -1) return null;
        int aliasOpenBrace = json.indexOf('{', aliasIdx);
        if (aliasOpenBrace == -1) return null;
        int aliasCloseBrace = json.indexOf('}', aliasOpenBrace);
        if (aliasCloseBrace == -1) return null;
        String aliasBlock = json.substring(aliasOpenBrace, aliasCloseBrace + 1);

        // Extract "name" from the aliasBlock substring
        int nameKeyIdx = aliasBlock.indexOf("\"name\"");
        if (nameKeyIdx == -1) return null;
        int nameColon = aliasBlock.indexOf(':', nameKeyIdx);
        int nameStart = aliasBlock.indexOf('"', nameColon) + 1;
        int nameEnd = aliasBlock.indexOf('"', nameStart);
        if (nameStart <= 0 || nameEnd <= nameStart) return null;
        String name = aliasBlock.substring(nameStart, nameEnd);

        // Extract "version" from the aliasBlock substring
        int versionKeyIdx = aliasBlock.indexOf("\"version\"");
        if (versionKeyIdx == -1) return null;
        int versionColon = aliasBlock.indexOf(':', versionKeyIdx);
        int versionStart = aliasBlock.indexOf('"', versionColon) + 1;
        int versionEnd = aliasBlock.indexOf('"', versionStart);
        if (versionStart <= 0 || versionEnd <= versionStart) return null;
        String ver = aliasBlock.substring(versionStart, versionEnd);

        if (name.isEmpty() || ver.isEmpty()) return null;
        return name + ":" + ver;
    }
}
```

- [ ] **Step 4: Run tests and confirm they pass**

```bash
cd gradle-plugin && ./gradlew test --tests "io.root.patcher.RootIoClientTest"
```
Expected: `BUILD SUCCESSFUL`, 5 tests passed.

- [ ] **Step 5: Commit**

```bash
cd /Users/idoberko2/root/rootio_patcher
rtk git add gradle-plugin/src/
rtk git commit -m "feat: add RootIoClient with manual JSON parsing"
```

---

## Task 5: `RootIoPatcherPlugin` (TDD)

**Files:**
- Create: `gradle-plugin/src/test/java/io/root/patcher/RootIoPatcherPluginFunctionalTest.java`
- Create: `gradle-plugin/src/main/java/io/root/patcher/RootIoPatcherPlugin.java`

- [ ] **Step 1: Write the failing functional tests**

Create `gradle-plugin/src/test/java/io/root/patcher/RootIoPatcherPluginFunctionalTest.java`:

```java
package io.root.patcher;

import com.sun.net.httpserver.HttpServer;
import org.gradle.testkit.runner.BuildResult;
import org.gradle.testkit.runner.GradleRunner;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.zip.ZipOutputStream;

import static org.junit.jupiter.api.Assertions.*;

class RootIoPatcherPluginFunctionalTest {

    @TempDir
    File projectDir;

    private HttpServer server;
    private int port;

    @BeforeEach
    void startServer() throws IOException {
        server = HttpServer.create(new InetSocketAddress(0), 0);
        port = server.getAddress().getPort();
        server.start();
    }

    @AfterEach
    void stopServer() {
        server.stop(0);
    }

    @Test
    void substitutesDepWhenPatchAvailable() throws IOException {
        setupServerResponse(200,
            "{\"patches\":[{" +
            "\"package_name\":\"io.test:my-lib\",\"version\":\"1.0.0\"," +
            "\"patch_alias\":{\"name\":\"io.root.io.test:my-lib\",\"version\":\"1.0.0-patched\"}," +
            "\"cve_ids\":[]}]," +
            "\"skipped\":[]}");
        writeProjectFiles();

        BuildResult result = GradleRunner.create()
            .withProjectDir(projectDir)
            .withPluginClasspath()
            .withGradleVersion("7.6")
            .withArguments("dependencies", "--configuration", "compileClasspath")
            .build();

        assertTrue(result.getOutput().contains("io.root.io.test:my-lib:1.0.0-patched"),
            "Expected patched coordinates in output:\n" + result.getOutput());
    }

    @Test
    void doesNotSubstituteWhenNoPatchAvailable() throws IOException {
        setupServerResponse(200, "{\"patches\":[],\"skipped\":[]}");
        writeProjectFiles();

        BuildResult result = GradleRunner.create()
            .withProjectDir(projectDir)
            .withPluginClasspath()
            .withGradleVersion("7.6")
            .withArguments("dependencies", "--configuration", "compileClasspath")
            .build();

        assertTrue(result.getOutput().contains("io.test:my-lib:1.0.0"),
            "Expected original coordinates in output:\n" + result.getOutput());
        assertFalse(result.getOutput().contains("io.root.io.test"),
            "Expected no substitution in output:\n" + result.getOutput());
    }

    @Test
    void reasonStringAppearsInDependencyInsight() throws IOException {
        setupServerResponse(200,
            "{\"patches\":[{" +
            "\"package_name\":\"io.test:my-lib\",\"version\":\"1.0.0\"," +
            "\"patch_alias\":{\"name\":\"io.root.io.test:my-lib\",\"version\":\"1.0.0-patched\"}," +
            "\"cve_ids\":[]}]," +
            "\"skipped\":[]}");
        writeProjectFiles();

        BuildResult result = GradleRunner.create()
            .withProjectDir(projectDir)
            .withPluginClasspath()
            .withGradleVersion("7.6")
            .withArguments("dependencyInsight", "--dependency", "io.test:my-lib",
                "--configuration", "compileClasspath")
            .build();

        assertTrue(result.getOutput().contains("Root.io security patch"),
            "Expected 'Root.io security patch' reason in dependencyInsight output:\n" + result.getOutput());
    }

    @Test
    void failsBuildWhenApiReturns500() throws IOException {
        setupServerResponse(500, "");
        writeProjectFiles();

        BuildResult result = GradleRunner.create()
            .withProjectDir(projectDir)
            .withPluginClasspath()
            .withGradleVersion("7.6")
            .withArguments("dependencies", "--configuration", "compileClasspath")
            .buildAndFail();

        assertTrue(result.getOutput().contains("500") || result.getOutput().contains("Root.io"),
            "Expected error message about HTTP 500 in output:\n" + result.getOutput());
    }

    // --- Helpers ---

    private void setupServerResponse(int status, String body) {
        server.createContext("/v3/analyze/maven", exchange -> {
            byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
            exchange.sendResponseHeaders(status, status == 200 ? bytes.length : -1);
            if (status == 200) {
                try (OutputStream os = exchange.getResponseBody()) {
                    os.write(bytes);
                }
            } else {
                exchange.getResponseBody().close();
            }
        });
    }

    private void writeProjectFiles() throws IOException {
        // Local Maven repo with fake artifacts so Gradle can resolve them
        File repoDir = new File(projectDir, "local-repo");
        createFakeArtifact(repoDir, "io.test", "my-lib", "1.0.0");
        createFakeArtifact(repoDir, "io.root.io.test", "my-lib", "1.0.0-patched");

        Files.writeString(new File(projectDir, "settings.gradle.kts").toPath(),
            "rootProject.name = \"test-project\"\n");

        Files.writeString(new File(projectDir, "build.gradle.kts").toPath(),
            "plugins {\n" +
            "    java\n" +
            "    id(\"io.root.patcher\")\n" +
            "}\n" +
            "repositories {\n" +
            "    maven { url = uri(\"" + repoDir.toURI() + "\") }\n" +
            "}\n" +
            "dependencies {\n" +
            "    implementation(\"io.test:my-lib:1.0.0\")\n" +
            "}\n" +
            "rootio {\n" +
            "    apiKey.set(\"test-key\")\n" +
            "    apiUrl.set(\"http://localhost:" + port + "\")\n" +
            "}\n");
    }

    /**
     * Creates a minimal Maven artifact (POM + empty valid JAR) in a local file repository.
     * Gradle needs a valid ZIP/JAR file to successfully resolve the artifact.
     */
    private void createFakeArtifact(File repoDir, String group, String artifact, String version)
            throws IOException {
        File dir = new File(repoDir, group.replace('.', '/') + "/" + artifact + "/" + version);
        dir.mkdirs();

        String pom = "<project><modelVersion>4.0.0</modelVersion>" +
            "<groupId>" + group + "</groupId>" +
            "<artifactId>" + artifact + "</artifactId>" +
            "<version>" + version + "</version>" +
            "<packaging>jar</packaging></project>";
        Files.writeString(new File(dir, artifact + "-" + version + ".pom").toPath(), pom);

        // Create a valid (empty) JAR — JARs are ZIP files; an empty ZIP is valid
        try (ZipOutputStream zos = new ZipOutputStream(
                new FileOutputStream(new File(dir, artifact + "-" + version + ".jar")))) {
            // intentionally empty
        }
    }
}
```

- [ ] **Step 2: Run to confirm the tests fail**

```bash
cd gradle-plugin && ./gradlew test --tests "io.root.patcher.RootIoPatcherPluginFunctionalTest"
```
Expected: `FAILED` — `RootIoPatcherPlugin` does not exist yet (compilation error).

- [ ] **Step 3: Implement `RootIoPatcherPlugin.java`**

Create `gradle-plugin/src/main/java/io/root/patcher/RootIoPatcherPlugin.java`:

```java
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
```

- [ ] **Step 4: Run tests and confirm they pass**

```bash
cd gradle-plugin && ./gradlew test --tests "io.root.patcher.RootIoPatcherPluginFunctionalTest"
```
Expected: `BUILD SUCCESSFUL`, 4 tests passed.

- [ ] **Step 5: Run the full test suite**

```bash
cd gradle-plugin && ./gradlew test
```
Expected: `BUILD SUCCESSFUL`, all tests passed (13 total: 4 DepCacheTest + 5 RootIoClientTest + 4 functional).

- [ ] **Step 6: Commit**

```bash
cd /Users/idoberko2/root/rootio_patcher
rtk git add gradle-plugin/src/
rtk git commit -m "feat: implement RootIoPatcherPlugin with eachDependency hook"
```

---

## Task 6: Final verification and local publish

**Files:** No new files.

- [ ] **Step 1: Run full build**

```bash
cd gradle-plugin && ./gradlew build
```
Expected: `BUILD SUCCESSFUL`, all tests pass.

- [ ] **Step 2: Publish to local Maven repository**

```bash
cd gradle-plugin && ./gradlew publishToMavenLocal
```
Expected: `BUILD SUCCESSFUL`. The plugin is now available at `~/.m2/repository/io/root/rootio-patcher-gradle-plugin/0.1.0/`.

- [ ] **Step 3: Verify the published artifacts exist**

```bash
ls ~/.m2/repository/io/root/rootio-patcher-gradle-plugin/0.1.0/
ls ~/.m2/repository/io/root/patcher/io.root.patcher.gradle.plugin/0.1.0/
```
Expected: first path contains the JAR and POM; second path contains the plugin marker POM (required so consumers can resolve `id("io.root.patcher")` via `mavenLocal()`).

- [ ] **Step 4: Commit any remaining changes**

```bash
cd /Users/idoberko2/root/rootio_patcher
rtk git status
```
If clean, nothing to commit. If there are uncommitted changes, commit them:
```bash
rtk git add -A
rtk git commit -m "chore: gradle plugin build verification"
```

---

## Testing Against Real Open-Source Projects

After `./gradlew publishToMavenLocal`, apply the plugin to any target Gradle project by following these steps.

### Step 1: Add the local Maven repo to plugin resolution

In the target project's `settings.gradle.kts`:

```kotlin
pluginManagement {
    repositories {
        mavenLocal()
        gradlePluginPortal()
    }
}
```

### Step 2: Apply the plugin

In the target project's `build.gradle.kts` (or in `subprojects {}` for multi-module):

```kotlin
plugins {
    id("io.root.patcher") version "0.1.0"
}
rootio {
    apiKey.set(System.getenv("ROOTIO_API_KEY"))
}
```

Set your API key:
```bash
export ROOTIO_API_KEY=<your-key>
```

### Step 3: Observe cold cache behaviour

```bash
./gradlew dependencies --configuration compileClasspath
```

On first run, the plugin makes one API call per unique dependency. Watch `.gradle/rootio-cache/` fill up.

### Step 4: Observe warm cache behaviour

Run the same command again immediately. No API calls — cache hits only.

### Step 5: Check for substitutions

```bash
./gradlew dependencies --configuration compileClasspath | grep "io.root\."
```

Any line containing `->` with `io.root.` is a substituted dependency.

For full detail on why a specific dep was substituted:
```bash
./gradlew dependencyInsight --dependency <group>:<artifact> --configuration compileClasspath
```

### Suggested projects to test against

| Project | Clone URL | Why useful |
|---|---|---|
| Spring PetClinic REST | `https://github.com/spring-petclinic/spring-petclinic-rest` | Spring Boot, single module, many transitive deps |
| Micronaut Starter | `https://github.com/micronaut-projects/micronaut-starter` | Multi-module, varied dep graph |
| OpenTelemetry Java | `https://github.com/open-telemetry/opentelemetry-java` | Large multi-module |
| Quarkus | `https://github.com/quarkusio/quarkus` | Massive dep graph — stress test cold cache overhead |