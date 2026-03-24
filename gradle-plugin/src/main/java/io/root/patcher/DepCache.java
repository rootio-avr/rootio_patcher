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
            // writeCache always writes without spaces, so this check is exact
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
