package ch.hippomaps.app

import java.net.URLDecoder
import java.net.URLEncoder
import java.util.Locale

/**
 * Map links of other apps and sites as links of hippomaps.ch, which the page opens:
 * `?at=lat,lon&n=name` a place, `?to=lat,lon&tn=…&from=…&mode=car|bike|walk` a route,
 * `?q=text` a search, `#map=zoom/lat/lon` a view.
 *
 * - `geo:47.37,8.54`, `geo:0,0?q=47.37,8.54(Name)`, `geo:47.37,8.54?q=Café` (RFC 5870 and Android)
 * - `google.navigation:q=47.37,8.54&mode=w`
 * - Google Maps: `/maps?q=`, `/maps/search/?api=1&query=`, `/maps/dir/?api=1&destination=`,
 *   `/maps/place/Name/@47.37,8.54,17z/data=…!3d47.37!4d8.54`, `/maps/@47.37,8.54,15z`
 * - OpenStreetMap: `?mlat=&mlon=`, `#map=15/47.37/8.54`, `/directions?route=a;b`, `/search?query=`
 * - Apple Maps: `?ll=&q=`, `?daddr=&saddr=&dirflg=`, `?q=`, `?address=`
 * - hippomaps.ch itself over http or www
 *
 * Short links (maps.app.goo.gl, goo.gl/maps) need the network: [isShort], then the link they lead
 * to through [convert]. Plain Kotlin, without android.net.Uri, for the unit tests.
 */
object MapLinks {
    private const val BASE = "https://hippomaps.ch/"

    /** the hippomaps.ch link of a map link; null when it names no place, route, search or view */
    fun convert(link: String): String? {
        val s = link.trim()
        val colon = s.indexOf(':')
        if (colon <= 0) return null
        return when (s.substring(0, colon).lowercase(Locale.ROOT)) {
            "geo" -> geo(s.substring(colon + 1))
            "google.navigation" -> navigation(s.substring(colon + 1))
            "http", "https" -> web(s)
            else -> null
        }
    }

    /** a short link: only its redirect tells where it leads */
    fun isShort(link: String): Boolean {
        val u = parse(link) ?: return false
        return u.host == "maps.app.goo.gl" || (u.host == "goo.gl" && u.path.startsWith("/maps")) ||
            (u.host == "g.co" && u.path.startsWith("/kgs"))
    }

    // ------------------------------------------------------------------ the schemes
    private fun geo(ssp: String): String? {
        val (head, query) = ssp.split('?', limit = 2).let { it[0] to it.getOrElse(1) { "" } }
        val p = params(query)
        val at = point(head.substringBefore(';').split(',').take(2).joinToString(","))
        val zoom = p["z"]?.toIntOrNull()?.coerceIn(1, 21)
        val q = p["q"]?.trim().orEmpty()
        if (q.isNotEmpty()) {
            labelled(q)?.let { (pt, name) -> return place(pt, name) }
            return search(q, at?.takeIf { !it.zero }, zoom ?: 15)
        }
        if (at == null || at.zero) return null
        return place(at, "")
    }

    private fun navigation(ssp: String): String? {
        val p = params(ssp.removePrefix("//"))
        val mode = when (p["mode"]) { "w" -> "walk"; "b" -> "bike"; else -> "car" }
        val q = (p["q"] ?: p["ll"])?.trim().orEmpty()
        labelled(q)?.let { (pt, name) -> return route(pt, name, null, mode) }
        return if (q.isNotEmpty()) search(q, null, null) else null
    }

    private fun web(link: String): String? {
        val u = parse(link) ?: return null
        val host = u.host.removePrefix("www.")
        return when {
            host == "hippomaps.ch" -> BASE + u.rest.removePrefix("/")
            host == "openstreetmap.org" || host == "osm.org" -> osm(u)
            host == "maps.apple.com" -> apple(u)
            google(u) -> googleMaps(u)
            else -> null
        }
    }

    // ------------------------------------------------------------------ Google Maps
    private val googleHost = Regex("""^(www\.|maps\.)?google\.[a-z]{2,3}(\.[a-z]{2})?$""")

    private fun google(u: Url) = googleHost.matches(u.host) &&
        (u.host.startsWith("maps.") || u.path == "/maps" || u.path.startsWith("/maps/"))

    private fun googleMaps(u: Url): String? {
        val p = u.params
        val segs = u.path.removePrefix("/maps").split('/').filter { it.isNotEmpty() }.map(::decodePath)
        val view = segs.firstNotNullOfOrNull(::atView)
        val pin = pinOf(u.path) // the place itself in data=…!3dLAT!4dLON
        val mode = when (p["travelmode"] ?: p["dirflg"]) {
            "walking", "w" -> "walk"
            "bicycling", "b" -> "bike"
            else -> "car"
        }
        // a route
        (p["destination"] ?: p["daddr"])?.trim()?.takeIf { it.isNotEmpty() }?.let { dest ->
            val from = (p["origin"] ?: p["saddr"])?.let(::labelled)
            labelled(dest)?.let { (pt, name) -> return route(pt, name, from, mode) }
            return search(dest, null, null)
        }
        if (segs.firstOrNull() == "dir") {
            val ends = segs.drop(1).filter { !it.startsWith("@") && !it.startsWith("data=") && !it.startsWith("am=") }
            val dest = ends.lastOrNull()?.trim().orEmpty()
            if (dest.isNotEmpty()) {
                val from = ends.dropLast(1).lastOrNull()?.let(::labelled)
                labelled(dest)?.let { (pt, name) -> return route(pt, name, from, mode) }
                return search(dest, null, null)
            }
        }
        // a place or a search
        (p["query"] ?: p["q"])?.trim()?.takeIf { it.isNotEmpty() }?.let { q ->
            labelled(q)?.let { (pt, name) -> return place(pt, name) }
            return search(q, view?.first, view?.second)
        }
        if (segs.firstOrNull() == "place" && segs.size > 1) {
            val name = segs[1]
            val at = pin ?: view?.first ?: labelled(name)?.first
            return if (at != null) place(at, if (labelled(name) != null) "" else name) else search(name, null, null)
        }
        if (segs.firstOrNull() == "search" && segs.size > 1 && !segs[1].startsWith("@")) {
            labelled(segs[1])?.let { (pt, name) -> return place(pt, name) }
            return search(segs[1], view?.first, view?.second)
        }
        pin?.let { return place(it, "") }
        p["ll"]?.let(::point)?.let { return mapView(it, p["z"]?.toIntOrNull() ?: 15) }
        p["center"]?.let(::point)?.let { return mapView(it, p["zoom"]?.toIntOrNull() ?: 15) }
        view?.let { return mapView(it.first, it.second) }
        return null
    }

    // "@47.37,8.54,15z" (or 1200m, 35a,90y,…: a zoom of 15 then)
    private fun atView(seg: String): Pair<Point, Int>? {
        if (!seg.startsWith("@")) return null
        val parts = seg.substring(1).split(',')
        val pt = point(parts.take(2).joinToString(",")) ?: return null
        val z = parts.getOrNull(2)?.takeIf { it.endsWith("z") }?.dropLast(1)?.toDoubleOrNull()?.toInt() ?: 15
        return pt to z.coerceIn(1, 21)
    }

    private val pinRe = Regex("""!3d(-?\d+(?:\.\d+)?)!4d(-?\d+(?:\.\d+)?)""")

    private fun pinOf(path: String): Point? =
        pinRe.findAll(path).lastOrNull()?.let { point("${it.groupValues[1]},${it.groupValues[2]}") }

    // ------------------------------------------------------------------ OpenStreetMap
    private fun osm(u: Url): String? {
        val p = u.params
        val frag = params(u.fragment)
        val view = frag["map"]?.split('/')?.takeIf { it.size >= 3 }?.let { m ->
            point("${m[1]},${m[2]}")?.let { it to (m[0].toIntOrNull() ?: 15) }
        }
        if (u.path.startsWith("/directions")) {
            val ends = p["route"]?.split(';').orEmpty()
            val to = ends.getOrNull(1)?.let(::point)
            if (to != null) {
                val mode = when {
                    p["engine"].orEmpty().contains("bicycle") || p["engine"].orEmpty().endsWith("_bike") -> "bike"
                    p["engine"].orEmpty().endsWith("_foot") -> "walk"
                    else -> "car"
                }
                return route(to, "", ends.getOrNull(0)?.let(::point)?.let { it to "" }, mode)
            }
        }
        if (u.path.startsWith("/search")) p["query"]?.trim()?.takeIf { it.isNotEmpty() }?.let { q ->
            labelled(q)?.let { (pt, name) -> return place(pt, name) }
            return search(q, view?.first, view?.second)
        }
        point("${p["mlat"]},${p["mlon"]}")?.let { return place(it, "") }
        view?.let { return mapView(it.first, it.second) }
        return null
    }

    // ------------------------------------------------------------------ Apple Maps
    private fun apple(u: Url): String? {
        val p = u.params
        val mode = when (p["dirflg"]) { "w" -> "walk"; else -> "car" }
        p["daddr"]?.trim()?.takeIf { it.isNotEmpty() }?.let { dest ->
            val from = p["saddr"]?.let(::labelled)
            labelled(dest)?.let { (pt, name) -> return route(pt, name, from, mode) }
            return search(dest, null, null)
        }
        val ll = (p["ll"] ?: p["sll"] ?: p["coordinate"])?.let(::point)
        val q = (p["q"] ?: p["address"] ?: p["name"])?.trim().orEmpty()
        if (ll != null) return place(ll, if (labelled(q) != null) "" else q)
        if (q.isNotEmpty()) {
            labelled(q)?.let { (pt, name) -> return place(pt, name) }
            return search(q, null, null)
        }
        return null
    }

    // ------------------------------------------------------------------ the links of hippomaps.ch
    private fun place(at: Point, name: String) =
        BASE + "?" + query("at" to at.text, "n" to name.trim())

    private fun route(to: Point, name: String, from: Pair<Point, String>?, mode: String) =
        BASE + "?" + query("from" to from?.first?.text.orEmpty(), "fn" to from?.second.orEmpty(),
            "to" to to.text, "tn" to name.trim(), "mode" to mode)

    private fun search(text: String, near: Point?, zoom: Int?) =
        BASE + "?" + query("q" to text.trim()) + (near?.let { "#map=${zoom ?: 15}/${it.text.replace(',', '/')}" } ?: "")

    private fun mapView(at: Point, zoom: Int) = BASE + "#map=${zoom.coerceIn(1, 21)}/${at.text.replace(',', '/')}"

    private fun query(vararg kv: Pair<String, String>) =
        kv.filter { it.second.isNotEmpty() }.joinToString("&") { (k, v) ->
            k + "=" + URLEncoder.encode(v, "UTF-8").replace("%2C", ",").replace("%2F", "/")
        }

    // ------------------------------------------------------------------ points and text
    class Point(val lat: Double, val lon: Double) {
        val zero get() = lat == 0.0 && lon == 0.0
        val text: String get() = "${fmt(lat)},${fmt(lon)}"
        private fun fmt(v: Double) = String.format(Locale.ROOT, "%.6f", v).trimEnd('0').trimEnd('.')
    }

    private val pointRe = Regex("""^\s*(-?\d{1,3}(?:\.\d+)?)\s*,\s*(-?\d{1,3}(?:\.\d+)?)\s*$""")

    /** "47.37,8.54" */
    fun point(s: String?): Point? {
        val m = pointRe.matchEntire(s ?: return null) ?: return null
        val lat = m.groupValues[1].toDouble()
        val lon = m.groupValues[2].toDouble()
        return if (lat in -90.0..90.0 && lon in -180.0..180.0) Point(lat, lon) else null
    }

    private val labelledRe = Regex("""^\s*(-?\d{1,3}(?:\.\d+)?)\s*,\s*(-?\d{1,3}(?:\.\d+)?)\s*(?:\((.*)\))?\s*$""")

    /** "47.37,8.54" or "47.37,8.54(Name)": a point and its name */
    private fun labelled(s: String): Pair<Point, String>? {
        val m = labelledRe.matchEntire(s) ?: return null
        val pt = point("${m.groupValues[1]},${m.groupValues[2]}") ?: return null
        return pt to m.groupValues[3].trim()
    }

    private fun params(q: String): Map<String, String> =
        q.split('&').filter { it.isNotEmpty() }.associate {
            val (k, v) = it.split('=', limit = 2).let { kv -> kv[0] to kv.getOrElse(1) { "" } }
            decode(k) to decode(v)
        }

    private fun decode(s: String) = try { URLDecoder.decode(s, "UTF-8") } catch (e: IllegalArgumentException) { s }

    // in a path a + is a +, except Google writes spaces of names as +
    private fun decodePath(s: String) = decode(s)

    /** an http(s) link in its parts; the host in lower case */
    class Url(val host: String, val path: String, val rawQuery: String, val fragment: String) {
        val params get() = params(rawQuery)
        // path, query and fragment as they came: for hippomaps.ch itself
        val rest get() = path + (if (rawQuery.isNotEmpty()) "?$rawQuery" else "") + (if (fragment.isNotEmpty()) "#$fragment" else "")
    }

    fun parse(link: String): Url? {
        val m = Regex("""^(?i:https?)://([^/?#]*)([^?#]*)(?:\?([^#]*))?(?:#(.*))?$""").matchEntire(link.trim()) ?: return null
        val authority = m.groupValues[1]
        if ('@' in authority || '\\' in link) return null // user info: the host would not be what it seems
        val host = authority.substringBefore(':').lowercase(Locale.ROOT)
        return Url(host, m.groupValues[2], m.groupValues[3], m.groupValues[4])
    }
}
