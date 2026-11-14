package ch.hippomaps.app

import android.hardware.SensorManager
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import kotlin.math.cos
import kotlin.math.sin

// The heading of Compass from rotation vectors of known poses of the phone: flat, upright, tilted,
// over the head, with the screen turned.
class HeadingTest {
    // a rotation by deg degrees about an axis of the world (counterclockwise looking at its tip), as x, y, z, w
    private fun about(ax: Int, deg: Double): DoubleArray {
        val h = Math.toRadians(deg) / 2
        val q = doubleArrayOf(0.0, 0.0, 0.0, cos(h))
        q[ax] = sin(h)
        return q
    }

    // a then b (b about the axes of the world)
    private fun then(a: DoubleArray, b: DoubleArray): DoubleArray {
        val (ax, ay, az, aw) = a
        val (bx, by, bz, bw) = b
        return doubleArrayOf(
            bw * ax + bx * aw + by * az - bz * ay,
            bw * ay - bx * az + by * aw + bz * ax,
            bw * az + bx * ay - by * ax + bz * aw,
            bw * aw - bx * ax - by * ay - bz * az,
        )
    }

    private fun heading(q: DoubleArray, rotation: Int = 0) =
        Heading.degrees(Heading.matrix(FloatArray(4) { q[it].toFloat() }), rotation)

    private fun near(want: Double, got: Double?) {
        val g = got ?: error("no heading, wanted $want")
        assertEquals("heading", 0.0, Heading.apart(want, g), 0.5)
    }

    private val X = 0
    private val Z = 2
    private val flat = doubleArrayOf(0.0, 0.0, 0.0, 1.0) // on a table, the top to north

    @Test
    fun flatOnATable() {
        near(0.0, heading(flat))
        near(90.0, heading(about(Z, -90.0))) // the top to east
        near(225.0, heading(about(Z, 135.0))) // to south-west
        // the sensor may leave w out
        near(0.0, Heading.degrees(Heading.matrix(floatArrayOf(0f, 0f, 0f)), 0))
        near(90.0, Heading.degrees(Heading.matrix(floatArrayOf(0f, 0f, -0.70710677f)), 0))
    }

    @Test
    fun upright() {
        val north = about(X, 90.0) // held up in front of the face, the back to north
        near(0.0, heading(north))
        near(270.0, heading(then(north, about(Z, 90.0)))) // turned to west
        near(135.0, heading(then(north, about(Z, -135.0))))
        // past upright, the screen a little down: still the way the back faces
        near(0.0, heading(about(X, 100.0)))
    }

    @Test
    fun tilted() {
        for (tilt in listOf(10.0, 30.0, 45.0, 60.0, 80.0)) {
            near(90.0, heading(then(about(X, tilt), about(Z, -90.0))))
            near(200.0, heading(then(about(X, tilt), about(Z, -200.0))))
        }
    }

    @Test
    fun tiltedSidewaysOrTurnedInItsPlane() {
        val Y = 1
        // flat, the top to east, tilted to a side about its long axis
        for (roll in listOf(-60.0, -20.0, -5.0, 5.0, 20.0, 60.0)) near(90.0, heading(then(about(Y, roll), about(Z, -90.0))))
        // held up, the back to north, turned in its own plane like a wheel (about the axis toward the face: south)
        val north = about(X, 90.0)
        for (turn in listOf(-30.0, -10.0, 10.0, 30.0)) near(0.0, heading(then(north, about(Y, turn))))
    }

    @Test
    fun overTheHead() {
        // lying on the back, the screen down to the face, the top of the phone to north
        near(0.0, heading(about(1, 180.0)))
        // tipped back halfway over the face: no heading; on its side with the screen not turned: none either
        assertNull(heading(about(X, 135.0)))
        assertNull(heading(about(1, 90.0)))
    }

    @Test
    fun screenTurned() {
        // turned counterclockwise into landscape (ROTATION_90), on a table: the right side of the phone is the top of
        // the screen
        near(0.0, heading(about(Z, 90.0), 1))
        near(270.0, heading(about(Z, 90.0), 0))
        near(0.0, heading(about(Z, -90.0), 3))
        near(180.0, heading(flat, 2))
        // landscape and upright, the back to east
        near(90.0, heading(then(then(about(2, 90.0), about(X, 90.0)), about(Z, -90.0)), 1))
    }

    @Test
    fun accuracy() {
        assertEquals(20, Heading.accuracy(floatArrayOf(0f, 0f, 0f, 1f, 0.349f), SensorManager.SENSOR_STATUS_ACCURACY_HIGH))
        assertEquals(10, Heading.accuracy(floatArrayOf(0f, 0f, 0f, 1f, -1f), SensorManager.SENSOR_STATUS_ACCURACY_HIGH))
        assertEquals(10, Heading.accuracy(floatArrayOf(0f, 0f, 0f, 1f), SensorManager.SENSOR_STATUS_ACCURACY_HIGH))
        assertEquals(35, Heading.accuracy(floatArrayOf(0f, 0f, 0f, 1f), SensorManager.SENSOR_STATUS_ACCURACY_LOW))
        assertEquals(60, Heading.accuracy(floatArrayOf(0f, 0f, 0f, 1f), SensorManager.SENSOR_STATUS_UNRELIABLE))
        assertEquals(90, Heading.accuracy(floatArrayOf(0f, 0f, 0f, 1f, 3f), SensorManager.SENSOR_STATUS_ACCURACY_HIGH))
    }

    @Test
    fun angles() {
        assertEquals(20.0, Heading.apart(350.0, 10.0), 1e-9)
        assertEquals(180.0, Heading.apart(0.0, 180.0), 1e-9)
        assertEquals(270.0, Heading.normal(-90.0), 1e-9)
        assertEquals(0.0, Heading.normal(720.0), 1e-9)
    }
}
