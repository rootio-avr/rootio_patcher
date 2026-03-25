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
            .withGradleVersion("9.4.1")
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
            .withGradleVersion("9.4.1")
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
            .withGradleVersion("9.4.1")
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
            .withGradleVersion("9.4.1")
            .withArguments("forceResolve")
            .buildAndFail();

        assertTrue(result.getOutput().contains("500") || result.getOutput().contains("Root.io"),
            "Expected error message about HTTP 500 in output:\n" + result.getOutput());
    }

    @Test
    void resolvesFromAutoRegisteredPkgRepo() throws IOException {
        // The plugin must auto-register {pkgUrl}/maven-patches so patched artifacts resolve
        // without the user needing to add the repository manually.
        setupServerResponse(200,
            "{\"patches\":[{" +
            "\"package_name\":\"io.test:my-lib\",\"version\":\"1.0.0\"," +
            "\"patch_alias\":{\"name\":\"io.root.io.test:my-lib\",\"version\":\"1.0.0-patched\"}," +
            "\"cve_ids\":[]}]," +
            "\"skipped\":[]}");

        // Original artifact in the project's own repo; patched artifact ONLY in the pkg repo.
        // The build script does NOT declare the pkg repo — the plugin must add it automatically.
        File repoDir = new File(projectDir, "local-repo");
        File pkgRepoDir = new File(projectDir, "pkg-repo");
        createFakeArtifact(repoDir, "io.test", "my-lib", "1.0.0");
        // Plugin appends /maven to pkgUrl, so the artifact must live in that subdirectory.
        createFakeArtifact(new File(pkgRepoDir, "maven"), "io.root.io.test", "my-lib", "1.0.0-patched");

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
            "    pkgUrl.set(\"" + pkgRepoDir.toURI().toString().replaceAll("/$", "") + "\")\n" +
            "}\n" +
            "tasks.register(\"forceResolve\") {\n" +
            "    doLast {\n" +
            "        configurations[\"compileClasspath\"].files\n" +
            "    }\n" +
            "}\n");

        BuildResult result = GradleRunner.create()
            .withProjectDir(projectDir)
            .withPluginClasspath()
            .withGradleVersion("9.4.1")
            .withArguments("forceResolve")
            .build();

        assertTrue(result.getOutput().contains("BUILD SUCCESSFUL"),
            "Expected patched artifact to resolve from auto-registered pkg repo:\n" + result.getOutput());
    }

    // Note: the empty-version skip (fix for HTTP 400 on BOM/Kotlin-plugin-managed deps) is
    // enforced in RootIoPatcherPlugin.java via `if (version == null || version.isEmpty()) return`.
    // A functional test that injects an empty version before the plugin's eachDependency hook
    // is not feasible: all available pre-hook mechanisms (init scripts, dependencySubstitution,
    // build-script resolutionStrategy) either fire after the plugin's rule or Gradle rejects
    // empty version strings before they reach the hook. The fix is verified manually against
    // real projects (e.g. kotlin-result) where the Kotlin Gradle plugin produces empty-version
    // deps for kotlin-stdlib.

    @Test
    void verboseLogsSubstitutionDetails() throws IOException {
        setupServerResponse(200,
            "{\"patches\":[{" +
            "\"package_name\":\"io.test:my-lib\",\"version\":\"1.0.0\"," +
            "\"patch_alias\":{\"name\":\"io.root.io.test:my-lib\",\"version\":\"1.0.0-patched\"}," +
            "\"cve_ids\":[]}]," +
            "\"skipped\":[]}");
        writeProjectFiles(true); // verbose=true

        BuildResult result = GradleRunner.create()
            .withProjectDir(projectDir)
            .withPluginClasspath()
            .withGradleVersion("9.4.1")
            .withArguments("dependencies", "--configuration", "compileClasspath")
            .build();

        assertTrue(result.getOutput().contains("[Root.io]"),
            "Expected [Root.io] verbose prefix in output:\n" + result.getOutput());
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
        writeProjectFiles(false);
    }

    private void writeProjectFiles(boolean verbose) throws IOException {
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
            (verbose ? "    verbose.set(true)\n" : "") +
            "}\n" +
            // forceResolve uses strict (non-lenient) resolution — fails the build if any dep
            // throws during eachDependency. The `dependencies` task uses lenient resolution
            // and only marks deps FAILED without failing the overall build.
            "tasks.register(\"forceResolve\") {\n" +
            "    doLast {\n" +
            "        configurations[\"compileClasspath\"].files\n" +
            "    }\n" +
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
