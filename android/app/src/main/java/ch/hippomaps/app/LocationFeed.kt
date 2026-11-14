package ch.hippomaps.app

import android.annotation.SuppressLint
import android.content.Context
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Bundle
import android.os.Looper
import org.json.JSONObject

/**
 * Positions for the page from the platform LocationManager (GPS and network providers, no Google
 * Play services). Runs while the page has a watch or a request open. [push] gets each position as
 * the JSON message the bridge expects ({type: "pos", ...}), [onLocation] the position itself.
 */
class LocationFeed(context: Context, private val onLocation: (Location) -> Unit = {}, private val push: (String) -> Unit) {

    private val lm = context.applicationContext.getSystemService(LocationManager::class.java)
    private var running = false
    private var lastGpsAt = 0L

    private val listener = object : LocationListener {
        override fun onLocationChanged(location: Location) = deliver(location, true)
        override fun onProviderEnabled(provider: String) = Unit
        override fun onProviderDisabled(provider: String) = Unit
        @Deprecated("Deprecated in Java")
        override fun onStatusChanged(provider: String?, status: Int, extras: Bundle?) = Unit
    }

    private var runningFine = false

    val isRunning get() = running

    /**
     * Listens to the providers the permissions allow, also to those turned off, so positions come
     * once the user turns location on. False when none of them is on now.
     */
    @SuppressLint("MissingPermission") // the caller checks the permissions
    fun start(fine: Boolean): Boolean {
        val lm = lm ?: return false
        if (running && fine && !runningFine) stop() // precise location allowed since: add GPS
        val providers = buildList {
            if (fine) add(LocationManager.GPS_PROVIDER)
            add(LocationManager.NETWORK_PROVIDER)
        }.filter { it in lm.allProviders }
        val enabled = providers.filter { try { lm.isProviderEnabled(it) } catch (e: Exception) { false } }
        if (running) return enabled.isNotEmpty()
        if (providers.isEmpty()) return false
        for (p in providers) {
            try {
                lm.requestLocationUpdates(p, 1000L, 0f, listener, Looper.getMainLooper())
            } catch (e: Exception) {
                // a provider can go away between the check and the request
            }
        }
        running = true
        runningFine = fine
        // the last known position at once; the page decides if it is recent enough
        enabled.mapNotNull { p -> try { lm.getLastKnownLocation(p) } catch (e: Exception) { null } }
            .maxByOrNull { it.time }?.let { deliver(it, false) }
        return enabled.isNotEmpty()
    }

    fun stop() {
        if (!running) return
        running = false
        try {
            lm?.removeUpdates(listener)
        } catch (_: Exception) {
        }
    }

    private fun deliver(l: Location, fresh: Boolean) {
        val now = System.currentTimeMillis()
        if (fresh && l.provider == LocationManager.GPS_PROVIDER) lastGpsAt = now
        // with GPS fixes coming in, the coarser network fixes would make the position jump
        if (fresh && l.provider != LocationManager.GPS_PROVIDER && now - lastGpsAt < 10_000) return
        val m = JSONObject()
            .put("type", "pos")
            .put("fresh", fresh)
            .put("lat", l.latitude)
            .put("lon", l.longitude)
            .put("acc", if (l.hasAccuracy()) l.accuracy.toDouble() else 1000.0)
            .put("t", if (fresh) now else l.time)
        if (l.hasAltitude()) m.put("alt", l.altitude)
        if (l.hasVerticalAccuracy()) m.put("altAcc", l.verticalAccuracyMeters.toDouble())
        if (l.hasBearing()) m.put("heading", l.bearing.toDouble())
        if (l.hasSpeed()) m.put("speed", l.speed.toDouble())
        onLocation(l)
        push(m.toString())
    }
}
