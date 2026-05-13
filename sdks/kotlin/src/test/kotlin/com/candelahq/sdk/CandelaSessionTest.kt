package com.candelahq.sdk

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotSame
import kotlin.test.assertTrue

class CandelaSessionTest {

    @Test
    fun `validateId accepts valid IDs`() {
        assertEquals("acme-corp", validateId("acme-corp"))
        assertEquals("run_42", validateId("run_42"))
        assertEquals("a.b.c", validateId("a.b.c"))
        assertEquals("A".repeat(128), validateId("A".repeat(128)))
    }

    @Test
    fun `validateId rejects empty`() {
        assertThrows<IllegalArgumentException> { validateId("") }
    }

    @Test
    fun `validateId rejects spaces`() {
        assertThrows<IllegalArgumentException> { validateId("has spaces") }
    }

    @Test
    fun `validateId rejects too long`() {
        assertThrows<IllegalArgumentException> { validateId("A".repeat(129)) }
    }

    @Test
    fun `DSL builder sets both IDs`() {
        val session = candelaSession {
            tenantId = "acme"
            jobId = "run-1"
        }
        val h = session.headers()
        assertEquals("acme", h["X-Candela-Tenant-Id"])
        assertEquals("run-1", h["X-Candela-Job-Id"])
        assertTrue(h["Baggage"]!!.contains("candela.tenant_id=acme"))
        assertTrue(h["Baggage"]!!.contains("candela.job_id=run-1"))
    }

    @Test
    fun `companion create works for Java interop`() {
        val session = CandelaSession.create(tenantId = "t1", jobId = "j1")
        val h = session.headers()
        assertEquals("t1", h["X-Candela-Tenant-Id"])
        assertEquals("j1", h["X-Candela-Job-Id"])
    }

    @Test
    fun `headers are fresh each call`() {
        val session = candelaSession { tenantId = "t1" }
        val h1 = session.headers()
        val h2 = session.headers()
        assertNotSame(h1, h2)
        assertEquals(h1, h2)
    }

    @Test
    fun `empty session produces no headers`() {
        val session = candelaSession {}
        val h = session.headers()
        assertFalse(h.containsKey("Baggage"))
    }

    @Test
    fun `rejects invalid on construction`() {
        assertThrows<IllegalArgumentException> {
            candelaSession { tenantId = "bad spaces!" }
        }
    }

    @Test
    fun `injectHeaders preserves existing baggage`() {
        val session = candelaSession { tenantId = "t1" }
        val headers = mutableMapOf("Baggage" to "existing=val")
        session.injectHeaders(headers)
        assertTrue(headers["Baggage"]!!.startsWith("existing=val,"))
        assertTrue(headers["Baggage"]!!.contains("candela.tenant_id=t1"))
    }
}
