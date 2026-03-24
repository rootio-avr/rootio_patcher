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
