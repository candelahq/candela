package com.candelahq.sdk

/**
 * Candela Enrichment SDK for Kotlin/JVM.
 *
 * Zero-dependency middleware for propagating tenant and job metadata
 * to Candela AI observability proxies via W3C Baggage headers.
 *
 * ```kotlin
 * val session = candelaSession {
 *     tenantId = "acme-corp"
 *     jobId = "training-v3"
 * }
 *
 * // With OkHttp
 * val client = OkHttpClient.Builder()
 *     .addInterceptor(session.okHttpInterceptor())
 *     .build()
 *
 * // With any HTTP client
 * val headers = session.headers()
 * ```
 */

private val ID_PATTERN = Regex("^[a-zA-Z0-9\\-._]{1,128}$")

/**
 * Validate a tenant or job ID against the allowed pattern.
 * @throws IllegalArgumentException if the ID is invalid.
 */
@JvmOverloads
fun validateId(value: String, name: String = "id"): String {
    require(ID_PATTERN.matches(value)) {
        "candela: invalid $name \"$value\" — must be 1-128 chars of [a-zA-Z0-9._-]"
    }
    return value
}

/**
 * Reusable session that generates enrichment headers for all requests.
 *
 * Construct via DSL:
 * ```kotlin
 * val session = candelaSession {
 *     tenantId = "acme"
 *     jobId = "run-42"
 * }
 * ```
 *
 * Or from Java:
 * ```java
 * CandelaSession session = CandelaSession.create("acme", "run-42");
 * ```
 */
class CandelaSession private constructor(
    val tenantId: String?,
    val jobId: String?,
) {
    init {
        tenantId?.let { validateId(it, "tenant_id") }
        jobId?.let { validateId(it, "job_id") }
    }

    /** Return a fresh map of enrichment headers. */
    fun headers(): Map<String, String> = buildMap {
        val parts = mutableListOf<String>()

        tenantId?.let {
            parts += "candela.tenant_id=$it"
            put("X-Candela-Tenant-Id", it)
        }

        jobId?.let {
            parts += "candela.job_id=$it"
            put("X-Candela-Job-Id", it)
        }

        if (parts.isNotEmpty()) {
            put("Baggage", parts.joinToString(","))
        }
    }

    /**
     * Inject enrichment headers into an existing mutable map.
     * Preserves existing Baggage entries.
     */
    fun injectHeaders(headers: MutableMap<String, String>): MutableMap<String, String> {
        val ours = headers()
        for ((k, v) in ours) {
            if (k == "Baggage") {
                val existing = headers[k]
                headers[k] = if (existing != null) "$existing,$v" else v
            } else {
                headers[k] = v
            }
        }
        return headers
    }

    /** DSL builder for [CandelaSession]. */
    class Builder {
        var tenantId: String? = null
        var jobId: String? = null

        fun build(): CandelaSession = CandelaSession(tenantId, jobId)
    }

    companion object {
        /** Create a session from Java. */
        @JvmStatic
        @JvmOverloads
        fun create(tenantId: String? = null, jobId: String? = null): CandelaSession =
            CandelaSession(tenantId, jobId)
    }
}

/** DSL function to create a [CandelaSession]. */
fun candelaSession(block: CandelaSession.Builder.() -> Unit): CandelaSession =
    CandelaSession.Builder().apply(block).build()
