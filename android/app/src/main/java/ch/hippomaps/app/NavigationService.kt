package ch.hippomaps.app

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.graphics.BitmapFactory
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat

/**
 * Runs while the page navigates: a foreground service of type location, so the page keeps getting
 * positions and speaking the turns with the screen off or another app in front. Shows the next
 * instruction in an ongoing notification; its Stop action ends the navigation in the page.
 */
class NavigationService : Service() {

    companion object {
        private const val TAG = "hippomaps"
        private const val CHANNEL = "navigation"
        private const val ID = 1
        private const val ACTION_STOP = "ch.hippomaps.app.STOP_NAVIGATION"
        private const val WAKE_MAX_MS = 6 * 60 * 60 * 1000L // a forgotten navigation lets the phone sleep after 6 h

        /** The running service, if any (same process as the activity). */
        var instance: NavigationService? = null
            private set

        /** Called by the Stop action; the activity ends the navigation in the page. */
        var onStopRequested: (() -> Unit)? = null

        /** The activity wants the service; false after [stop]. */
        private var wanted = false
        /** Starts asked for that have not reached onStartCommand yet. */
        private var starting = 0

        fun start(context: Context) {
            ContextCompat.startForegroundService(context, Intent(context, NavigationService::class.java))
            wanted = true
            starting++
        }

        fun stop(context: Context) {
            wanted = false
            // stopped before it called startForeground, the app would crash ("did not then call
            // Service.startForeground"): a service still starting stops itself in onStartCommand
            if (starting == 0) context.stopService(Intent(context, NavigationService::class.java))
        }
    }

    private var wakeLock: PowerManager.WakeLock? = null
    private val largeIcon by lazy { BitmapFactory.decodeResource(resources, R.drawable.hm_icon) }
    private var dist = ""
    private var instr = ""

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        instance = this
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(
            NotificationChannel(CHANNEL, getString(R.string.nav_channel), NotificationManager.IMPORTANCE_LOW).apply {
                setShowBadge(false)
            }
        )
        // the page needs the processor for its timers and the voice while the screen is off
        wakeLock = getSystemService(PowerManager::class.java)
            .newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "hippomaps:navigation")
            .apply { setReferenceCounted(false); acquire(WAKE_MAX_MS) }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            // the page ends its navigation; the service and its notification go at once
            onStopRequested?.invoke()
            if (starting == 0) stopSelf()
            return START_NOT_STICKY
        }
        if (starting > 0) starting--
        try {
            ServiceCompat.startForeground(
                this, ID, build(),
                if (Build.VERSION.SDK_INT >= 29) ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION else 0
            )
        } catch (e: Exception) {
            // no location permission, or started from the background: navigation then only runs in front
            Log.w(TAG, "navigation service not started: $e")
            stopSelf()
            return START_NOT_STICKY
        }
        if (!wanted && starting == 0) stopSelf() // the navigation ended while the service was starting
        return START_NOT_STICKY
    }

    fun update(dist: String, instr: String) {
        if (dist == this.dist && instr == this.instr) return
        this.dist = dist
        this.instr = instr
        repost()
    }

    fun repost() {
        try {
            getSystemService(NotificationManager::class.java).notify(ID, build())
        } catch (e: SecurityException) {
            // notifications not allowed: the service runs on without them
        }
    }

    private fun build(): Notification {
        val open = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )
        val stop = PendingIntent.getService(
            this, 1,
            Intent(this, NavigationService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )
        val title = when {
            dist.isNotBlank() && instr.isNotBlank() -> "$dist · $instr"
            instr.isNotBlank() -> instr
            dist.isNotBlank() -> dist
            else -> getString(R.string.nav_running)
        }
        return NotificationCompat.Builder(this, CHANNEL)
            .setSmallIcon(R.drawable.ic_stat_hippo)
            .setLargeIcon(largeIcon)
            .setColor(ContextCompat.getColor(this, R.color.brand))
            .setContentTitle(title)
            .setContentText(getString(R.string.nav_running))
            .setContentIntent(open)
            .addAction(0, getString(R.string.nav_stop), stop)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .setShowWhen(false)
            .setCategory(NotificationCompat.CATEGORY_NAVIGATION)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .build()
    }

    override fun onTaskRemoved(rootIntent: Intent?) {
        // the app swiped away: navigation ends with it
        onStopRequested?.invoke()
        stopSelf()
    }

    override fun onDestroy() {
        instance = null
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
        super.onDestroy()
    }
}
