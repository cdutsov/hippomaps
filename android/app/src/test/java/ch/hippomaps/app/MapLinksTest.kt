package ch.hippomaps.app

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class MapLinksTest {
    private fun c(s: String) = MapLinks.convert(s)
    private val h = "https://hippomaps.ch/"

    @Test
    fun geo() {
        assertEquals("$h?at=47.3769,8.5417", c("geo:47.3769,8.5417"))
        assertEquals("$h?at=47.3769,8.5417", c("geo:47.3769,8.5417;u=35?z=17"))
        assertEquals("$h?at=47.3769,8.5417&n=Zurich+HB", c("geo:0,0?q=47.3769,8.5417(Zurich+HB)"))
        assertEquals("$h?at=47.3769,8.5417&n=Zurich+HB", c("geo:0,0?q=47.3769,8.5417(Zurich%20HB)"))
        assertEquals("$h?q=Bahnhofstrasse+10,+Z%C3%BCrich", c("geo:0,0?q=Bahnhofstrasse%2010%2C%20Z%C3%BCrich"))
        assertEquals("$h?q=caf%C3%A9#map=15/47.3769/8.5417", c("geo:47.3769,8.5417?q=caf%C3%A9"))
        assertNull(c("geo:0,0"))
        assertNull(c("geo:91,8"))
    }

    @Test
    fun navigation() {
        assertEquals("$h?to=47.3769,8.5417&mode=walk", c("google.navigation:q=47.3769,8.5417&mode=w"))
        assertEquals("$h?q=Bern", c("google.navigation:q=Bern"))
    }

    @Test
    fun google() {
        assertEquals("$h?at=47.3769,8.5417", c("https://www.google.com/maps?q=47.3769,8.5417"))
        assertEquals("$h?at=47.3769,8.5417", c("https://maps.google.com/?q=47.3769,8.5417"))
        assertEquals("$h?q=Technopark+Z%C3%BCrich", c("https://www.google.com/maps/search/?api=1&query=Technopark+Z%C3%BCrich"))
        assertEquals("$h?to=47.3769,8.5417&mode=bike", c("https://www.google.com/maps/dir/?api=1&destination=47.3769,8.5417&travelmode=bicycling"))
        assertEquals("$h?from=47.5,8.2&to=47.3769,8.5417&mode=car", c("https://www.google.ch/maps/dir/47.5,8.2/47.3769,8.5417/@47.4,8.3,11z"))
        assertEquals("$h?at=47.391,8.5155&n=Technopark+Z%C3%BCrich",
            c("https://www.google.com/maps/place/Technopark+Z%C3%BCrich/@47.3909,8.5129,17z/data=!3m1!4b1!4m6!3m5!1s0x0:0x0!8m2!3d47.391!4d8.5155!16s"))
        assertEquals("$h?at=47.3909,8.5129&n=Technopark", c("https://www.google.com/maps/place/Technopark/@47.3909,8.5129,17z"))
        assertEquals("$h#map=14/47.3909/8.5129", c("https://www.google.com/maps/@47.3909,8.5129,14z"))
        assertEquals("$h?q=pizza#map=13/47.39/8.51", c("https://www.google.com/maps/search/pizza/@47.39,8.51,13z"))
        assertNull(c("https://www.google.com/search?q=maps"))
        assertNull(c("https://www.google.com/maps"))
    }

    @Test
    fun osm() {
        assertEquals("$h?at=47.3769,8.5417", c("https://www.openstreetmap.org/?mlat=47.3769&mlon=8.5417#map=17/47.3769/8.5417"))
        assertEquals("$h#map=12/47.37/8.54", c("https://www.openstreetmap.org/#map=12/47.37/8.54"))
        assertEquals("$h#map=12/47.37/8.54", c("https://www.openstreetmap.org/node/123#map=12/47.37/8.54"))
        assertEquals("$h?from=47.5,8.2&to=47.37,8.54&mode=walk",
            c("https://www.openstreetmap.org/directions?engine=fossgis_osrm_foot&route=47.5%2C8.2%3B47.37%2C8.54"))
        assertEquals("$h?q=Riniken", c("https://www.openstreetmap.org/search?query=Riniken"))
    }

    @Test
    fun apple() {
        assertEquals("$h?at=47.3769,8.5417&n=Zurich+HB", c("https://maps.apple.com/?ll=47.3769,8.5417&q=Zurich%20HB"))
        assertEquals("$h?to=47.3769,8.5417&mode=walk", c("https://maps.apple.com/?daddr=47.3769,8.5417&dirflg=w"))
        assertEquals("$h?q=Bern", c("https://maps.apple.com/?q=Bern"))
    }

    @Test
    fun ours() {
        assertEquals("$h?at=47.3769,8.5417", c("http://www.hippomaps.ch/?at=47.3769,8.5417"))
        assertEquals("$h#map=12/47.37/8.54", c("http://hippomaps.ch/#map=12/47.37/8.54"))
        assertNull(c("https://evil.example@hippomaps.ch/"))
        assertNull(c("https://example.com/maps?q=47,8"))
        assertNull(c("javascript:alert(1)"))
    }

    @Test
    fun short() {
        assertTrue(MapLinks.isShort("https://maps.app.goo.gl/AbCdEf"))
        assertTrue(MapLinks.isShort("https://goo.gl/maps/AbCdEf"))
        assertFalse(MapLinks.isShort("https://goo.gl/AbCdEf"))
        assertFalse(MapLinks.isShort("https://www.google.com/maps?q=1,2"))
    }
}
