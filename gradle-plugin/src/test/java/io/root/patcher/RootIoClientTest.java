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
