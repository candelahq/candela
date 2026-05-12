package com.candelahq.sdk;

import java.net.http.HttpRequest;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.StringJoiner;
import java.util.regex.Pattern;

/**
 * Candela Enrichment SDK for Java.
 *
 * <p>Zero-dependency utility for propagating tenant and job metadata to Candela
 * AI observability proxies via W3C Baggage headers.
 *
 * <h3>Usage with HttpClient:</h3>
 * <pre>{@code
 * CandelaSession session = new CandelaSession.Builder()
 *     .tenantId("acme-corp")
 *     .jobId("training-v3")
 *     .build();
 *
 * HttpRequest.Builder builder = HttpRequest.newBuilder()
 *     .uri(URI.create("http://localhost:8080/v1/chat/completions"));
 * session.injectHeaders(builder);
 * HttpRequest request = builder.POST(...).build();
 * }</pre>
 *
 * <h3>Usage with OkHttp Interceptor:</h3>
 * <pre>{@code
 * OkHttpClient client = new OkHttpClient.Builder()
 *     .addInterceptor(chain -> {
 *         Request original = chain.request();
 *         Request.Builder builder = original.newBuilder();
 *         session.headers().forEach(builder::header);
 *         return chain.proceed(builder.build());
 *     })
 *     .build();
 * }</pre>
 */
public final class CandelaSession {

    private static final Pattern ID_PATTERN =
            Pattern.compile("^[a-zA-Z0-9\\-._]{1,128}$");

    private final String tenantId;
    private final String jobId;

    private CandelaSession(String tenantId, String jobId) {
        this.tenantId = tenantId;
        this.jobId = jobId;
    }

    /**
     * Validate a tenant or job ID against the allowed pattern.
     *
     * @throws IllegalArgumentException if the ID is invalid
     */
    public static String validateId(String value, String name) {
        if (value == null || !ID_PATTERN.matcher(value).matches()) {
            throw new IllegalArgumentException(
                    "candela: invalid " + name + " \"" + value
                            + "\" — must be 1-128 chars of [a-zA-Z0-9._-]");
        }
        return value;
    }

    /**
     * Return enrichment headers as a Map. A fresh Map is returned on each call.
     */
    public Map<String, String> headers() {
        Map<String, String> h = new LinkedHashMap<>();
        StringJoiner baggage = new StringJoiner(",");

        if (tenantId != null) {
            baggage.add("candela.tenant_id=" + tenantId);
            h.put("X-Candela-Tenant-Id", tenantId);
        }
        if (jobId != null) {
            baggage.add("candela.job_id=" + jobId);
            h.put("X-Candela-Job-Id", jobId);
        }
        if (baggage.length() > 0) {
            h.put("Baggage", baggage.toString());
        }
        return h;
    }

    /**
     * Inject enrichment headers into a {@link HttpRequest.Builder}.
     */
    public void injectHeaders(HttpRequest.Builder builder) {
        headers().forEach(builder::header);
    }

    /** Builder for {@link CandelaSession}. */
    public static final class Builder {
        private String tenantId;
        private String jobId;

        public Builder tenantId(String tenantId) {
            this.tenantId = validateId(tenantId, "tenant_id");
            return this;
        }

        public Builder jobId(String jobId) {
            this.jobId = validateId(jobId, "job_id");
            return this;
        }

        public CandelaSession build() {
            return new CandelaSession(tenantId, jobId);
        }
    }
}
