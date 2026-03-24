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
