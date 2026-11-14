package ch.hippomaps.app

import android.content.Context
import android.hardware.GeomagneticField
import android.hardware.Sensor
import android.hardware.SensorEvent
import android.hardware.SensorEventListener
import android.hardware.SensorManager
import android.location.Location
import android.os.SystemClock
import org.json.JSONObject
import kotlin.math.abs
import kotlin.math.atan2
import kotlin.math.cos
import kotlin.math.exp
import kotlin.math.max
import kotlin.math.roundToInt
import kotlin.math.sin
import kotlin.math.sqrt

/**
 * The heading of the phone for the page: where the top of the screen points, in degrees clockwise
 * from true north, from the rotation sensor (gyroscope, accelerometer and magnetometer fused by
 * Android; the magnetometer and accelerometer alone where there is no gyroscope). Runs while the
 * page asks and the app is in front. [push] gets each heading as the message the bridge expects
 * ({type: "heading", h, acc}); [rotation] gives the turn of the screen (Surface.ROTATION_*).
 */
class Compass(context: Context, private val rotation: () -> Int, private val push: (String) -> Unit) {

    private val sm = context.applicationContext.getSystemService(SensorManager::class.java)
    private val sensor = sm?.getDefaultSensor(Sensor.TYPE_ROTATION_VECTOR)
        ?: sm?.getDefaultSensor(Sensor.TYPE_GEOMAGNETIC_ROTATION_VECTOR)
    private var running = false
    private var status = SensorManager.SENSOR_STATUS_ACCURACY_MEDIUM
    private var declination = 0.0 // east of true north, where the phone last was
    private var declinationAt: Location? = null

    // the heading smoothed as a vector (a mean of 359 and 1 degrees is 0), and what the page got last
    private var sx = 0.0
    private var sy = 0.0
    private var smoothed = false
    private var lastEventAt = 0L
    private var sentH = Double.NaN
    private var sentAcc = -1
    private var sentAt = 0L

    val available get() = sensor != null

    private val listener = object : SensorEventListener {
        override fun onSensorChanged(e: SensorEvent) = onRotation(e)
        override fun onAccuracyChanged(s: Sensor, accuracy: Int) {
            status = accuracy
        }
    }

    /** False when the phone has no sensor for it. */
    fun start(): Boolean {
        val s = sensor ?: return false
        if (running) return true
        smoothed = false
        sentH = Double.NaN
        sentAcc = -1
        running = try {
            sm?.registerListener(listener, s, SensorManager.SENSOR_DELAY_GAME) == true
        } catch (e: Exception) {
            false
        }
        return running
    }

    fun stop() {
        if (!running) return
        running = false
        try {
            sm?.unregisterListener(listener)
        } catch (_: Exception) {
        }
    }

    /** The declination of the magnetic field where the phone is: north of the sensor to true north. */
    fun at(l: Location) {
        val before = declinationAt
        if (before != null && before.distanceTo(l) < 10_000f) return
        declinationAt = l
        declination = try {
            GeomagneticField(l.latitude.toFloat(), l.longitude.toFloat(), l.altitude.toFloat(), System.currentTimeMillis()).declination.toDouble()
        } catch (e: Exception) {
            0.0
        }
    }

    private fun onRotation(e: SensorEvent) {
        val r = Heading.matrix(e.values)
        val magnetic = Heading.degrees(r, rotation()) ?: return
        val h = Math.toRadians(magnetic + declination)
        // smoothing of about a tenth of a second, whatever the rate of the sensor
        val now = SystemClock.elapsedRealtime()
        val a = if (!smoothed) 1.0 else 1 - exp(-(now - lastEventAt).coerceIn(1, 500) / SMOOTH_MS)
        lastEventAt = now
        sx += a * (sin(h) - sx)
        sy += a * (cos(h) - sy)
        smoothed = true
        val heading = Heading.normal(Math.toDegrees(atan2(sx, sy)))
        val acc = Heading.accuracy(e.values, status)
        // the page needs no more than 25 headings a second, and none for a change it would not show
        val turned = sentH.isNaN() || Heading.apart(heading, sentH) >= 0.5
        if ((turned || acc != sentAcc) && now - sentAt >= MIN_GAP_MS) {
            sentH = heading
            sentAcc = acc
            sentAt = now
            push(JSONObject().put("type", "heading").put("h", Math.round(heading * 10) / 10.0).put("acc", acc).toString())
        }
    }

    private companion object {
        const val SMOOTH_MS = 100.0
        const val MIN_GAP_MS = 40L
    }
}

/** The math of [Compass], apart from Android for the tests. */
object Heading {
    /**
     * The rotation matrix (row major) of a rotation vector of the sensors: device axes to the axes
     * of the world (east, north, up); as SensorManager.getRotationMatrixFromVector.
     */
    fun matrix(v: FloatArray): DoubleArray {
        val x = v[0].toDouble()
        val y = v[1].toDouble()
        val z = v[2].toDouble()
        val w = if (v.size >= 4) v[3].toDouble() else sqrt(max(0.0, 1 - x * x - y * y - z * z))
        return doubleArrayOf(
            1 - 2 * (y * y + z * z), 2 * (x * y - z * w), 2 * (x * z + y * w),
            2 * (x * y + z * w), 1 - 2 * (x * x + z * z), 2 * (y * z - x * w),
            2 * (x * z - y * w), 2 * (y * z + x * w), 1 - 2 * (x * x + y * y),
        )
    }

    /**
     * Where the top of the screen points, in degrees clockwise from the north of [r], with the
     * screen turned by [rotation] quarter turns against the phone (Surface.ROTATION_*). Held flat,
     * the top of the screen; held upright, the way the back of the phone faces; tilted between,
     * both point the same way and their sum stays clear. Null when the two cancel out (the phone
     * over the head with the screen down).
     */
    fun degrees(r: DoubleArray, rotation: Int): Double? {
        // the top of the screen in the axes of the phone: y, x, -y, -x; its east and north parts
        val (upE, upN) = when (rotation and 3) {
            0 -> r[1] to r[4]
            1 -> r[0] to r[3]
            2 -> -r[1] to -r[4]
            else -> -r[0] to -r[3]
        }
        // and the back of the phone: -z
        val e = upE - r[2]
        val n = upN - r[5]
        if (e * e + n * n < 0.01) return null
        return normal(Math.toDegrees(atan2(e, n)))
    }

    /**
     * How far off the heading may be, in whole degrees: the estimate of the sensor where it gives
     * one, else by the accuracy it reports.
     */
    fun accuracy(v: FloatArray, status: Int): Int {
        if (v.size >= 5 && v[4] > 0f) return Math.toDegrees(v[4].toDouble()).roundToInt().coerceIn(5, 90)
        return when (status) {
            SensorManager.SENSOR_STATUS_ACCURACY_HIGH -> 10
            SensorManager.SENSOR_STATUS_ACCURACY_MEDIUM -> 20
            SensorManager.SENSOR_STATUS_ACCURACY_LOW -> 35
            else -> 60
        }
    }

    fun normal(d: Double): Double = ((d % 360) + 360) % 360

    /** Degrees between two headings, 0 to 180. */
    fun apart(a: Double, b: Double): Double = abs(((a - b) % 360 + 540) % 360 - 180)
}
