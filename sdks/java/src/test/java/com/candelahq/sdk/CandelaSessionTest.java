package com.candelahq.sdk;

import org.junit.jupiter.api.Test;
import java.util.Map;
import static org.junit.jupiter.api.Assertions.*;

class CandelaSessionTest {

    @Test
    void validateIdAcceptsValid() {
        assertEquals("acme-corp", CandelaSession.validateId("acme-corp", "test"));
        assertEquals("run_42", CandelaSession.validateId("run_42", "test"));
        assertEquals("a.b.c", CandelaSession.validateId("a.b.c", "test"));
    }

    @Test
    void validateIdRejectsEmpty() {
        assertThrows(IllegalArgumentException.class,
                () -> CandelaSession.validateId("", "test"));
    }

    @Test
    void validateIdRejectsSpaces() {
        assertThrows(IllegalArgumentException.class,
                () -> CandelaSession.validateId("has spaces", "test"));
    }

    @Test
    void validateIdRejectsTooLong() {
        assertThrows(IllegalArgumentException.class,
                () -> CandelaSession.validateId("A".repeat(129), "test"));
    }

    @Test
    void headersBothIds() {
        CandelaSession session = new CandelaSession.Builder()
                .tenantId("acme")
                .jobId("run-1")
                .build();

        Map<String, String> h = session.headers();
        assertEquals("acme", h.get("X-Candela-Tenant-Id"));
        assertEquals("run-1", h.get("X-Candela-Job-Id"));
        assertTrue(h.get("Baggage").contains("candela.tenant_id=acme"));
        assertTrue(h.get("Baggage").contains("candela.job_id=run-1"));
    }

    @Test
    void headersAreFresh() {
        CandelaSession session = new CandelaSession.Builder()
                .tenantId("t1")
                .build();

        Map<String, String> h1 = session.headers();
        Map<String, String> h2 = session.headers();
        assertNotSame(h1, h2);
        assertEquals(h1, h2);
    }

    @Test
    void builderRejectsInvalid() {
        assertThrows(IllegalArgumentException.class,
                () -> new CandelaSession.Builder().tenantId("bad spaces!"));
    }

    @Test
    void headersEmptyWithNoIds() {
        CandelaSession session = new CandelaSession.Builder().build();
        Map<String, String> h = session.headers();
        assertFalse(h.containsKey("Baggage"));
    }
}
