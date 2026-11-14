// The declination of the magnetic field of the earth, in degrees east of true north, by the World Magnetic Model 2025
// (WMM2025, for 2025.0 to 2030.0; NOAA NCEI and the British Geological Survey), for the compass of a phone browser,
// which gives the heading from magnetic north. The coefficients are those of WMM.COF of WMM2025: degree, order, g, h
// (nT) and their change per year. tests/wmm.test.mjs checks the test values of NCEI.
const EPOCH = 2025.0;
const COF = [
  [1,0,-29351.8,0,12,0], [1,1,-1410.8,4545.4,9.7,-21.5], [2,0,-2556.6,0,-11.6,0], [2,1,2951.1,-3133.6,-5.2,-27.7], [2,2,1649.3,-815.1,-8,-12.1], [3,0,1361,0,-1.3,0],
  [3,1,-2404.1,-56.6,-4.2,4], [3,2,1243.8,237.5,0.4,-0.3], [3,3,453.6,-549.5,-15.6,-4.1], [4,0,895,0,-1.6,0], [4,1,799.5,278.6,-2.4,-1.1], [4,2,55.7,-133.9,-6,4.1],
  [4,3,-281.1,212,5.6,1.6], [4,4,12.1,-375.6,-7,-4.4], [5,0,-233.2,0,0.6,0], [5,1,368.9,45.4,1.4,-0.5], [5,2,187.2,220.2,0,2.2], [5,3,-138.7,-122.9,0.6,0.4],
  [5,4,-142,43,2.2,1.7], [5,5,20.9,106.1,0.9,1.9], [6,0,64.4,0,-0.2,0], [6,1,63.8,-18.4,-0.4,0.3], [6,2,76.9,16.8,0.9,-1.6], [6,3,-115.7,48.8,1.2,-0.4],
  [6,4,-40.9,-59.8,-0.9,0.9], [6,5,14.9,10.9,0.3,0.7], [6,6,-60.7,72.7,0.9,0.9], [7,0,79.5,0,0,0], [7,1,-77,-48.9,-0.1,0.6], [7,2,-8.8,-14.4,-0.1,0.5],
  [7,3,59.3,-1,0.5,-0.8], [7,4,15.8,23.4,-0.1,0], [7,5,2.5,-7.4,-0.8,-1], [7,6,-11.1,-25.1,-0.8,0.6], [7,7,14.2,-2.3,0.8,-0.2], [8,0,23.2,0,-0.1,0],
  [8,1,10.8,7.1,0.2,-0.2], [8,2,-17.5,-12.6,0,0.5], [8,3,2,11.4,0.5,-0.4], [8,4,-21.7,-9.7,-0.1,0.4], [8,5,16.9,12.7,0.3,-0.5], [8,6,15,0.7,0.2,-0.6],
  [8,7,-16.8,-5.2,0,0.3], [8,8,0.9,3.9,0.2,0.2], [9,0,4.6,0,0,0], [9,1,7.8,-24.8,-0.1,-0.3], [9,2,3,12.2,0.1,0.3], [9,3,-0.2,8.3,0.3,-0.3],
  [9,4,-2.5,-3.3,-0.3,0.3], [9,5,-13.1,-5.2,0,0.2], [9,6,2.4,7.2,0.3,-0.1], [9,7,8.6,-0.6,-0.1,-0.2], [9,8,-8.7,0.8,0.1,0.4], [9,9,-12.9,10,-0.1,0.1],
  [10,0,-1.3,0,0.1,0], [10,1,-6.4,3.3,0,0], [10,2,0.2,0,0.1,0], [10,3,2,2.4,0.1,-0.2], [10,4,-1,5.3,0,0.1], [10,5,-0.6,-9.1,-0.3,-0.1],
  [10,6,-0.9,0.4,0,0.1], [10,7,1.5,-4.2,-0.1,0], [10,8,0.9,-3.8,-0.1,-0.1], [10,9,-2.7,0.9,0,0.2], [10,10,-3.9,-9.1,0,0], [11,0,2.9,0,0,0],
  [11,1,-1.5,0,0,0], [11,2,-2.5,2.9,0,0.1], [11,3,2.4,-0.6,0,0], [11,4,-0.6,0.2,0,0.1], [11,5,-0.1,0.5,-0.1,0], [11,6,-0.6,-0.3,0,0],
  [11,7,-0.1,-1.2,0,0.1], [11,8,1.1,-1.7,-0.1,0], [11,9,-1,-2.9,-0.1,0], [11,10,-0.2,-1.8,-0.1,0], [11,11,2.6,-2.3,-0.1,0], [12,0,-2,0,0,0],
  [12,1,-0.2,-1.3,0,0], [12,2,0.3,0.7,0,0], [12,3,1.2,1,0,-0.1], [12,4,-1.3,-1.4,0,0.1], [12,5,0.6,0,0,0], [12,6,0.6,0.6,0.1,0],
  [12,7,0.5,-0.1,0,0], [12,8,-0.1,0.8,0,0], [12,9,-0.4,0.1,0,0], [12,10,-0.2,-1,-0.1,0], [12,11,-1.3,0.1,0,0], [12,12,-0.7,0.2,-0.1,-0.1],
];
const N = 12;
const A = 6371.2; // the radius of the model, km
const WGS_A = 6378.137, WGS_F = 1 / 298.257223563, E2 = WGS_F * (2 - WGS_F);
const rad = Math.PI / 180;

// the year with its fraction (2026.5 in the middle of 2026)
export function decimalYear(date = new Date()) {
  const y = date.getUTCFullYear(), t0 = Date.UTC(y, 0, 1), t1 = Date.UTC(y + 1, 0, 1);
  return y + (date.getTime() - t0) / (t1 - t0);
}

// declination at a geodetic latitude and longitude (degrees), height above the ellipsoid (km) and decimal year
export function declination(lat, lon, year = decimalYear(), heightKm = 0) {
  const dt = year - EPOCH;
  // geodetic to geocentric
  const phi = lat * rad, sinPhi = Math.sin(phi), cosPhi = Math.cos(phi);
  const rc = WGS_A / Math.sqrt(1 - E2 * sinPhi * sinPhi);
  const p = (rc + heightKm) * cosPhi, z = (rc * (1 - E2) + heightKm) * sinPhi;
  const r = Math.hypot(p, z), phiC = Math.asin(z / r);
  // Schmidt semi-normalized Legendre functions of the colatitude and their derivatives
  const c = Math.sin(phiC), s = Math.cos(phiC);
  const P = [], dP = [];
  for (let n = 0; n <= N; n++) { P.push(new Float64Array(n + 1)); dP.push(new Float64Array(n + 1)); }
  P[0][0] = 1;
  for (let n = 1; n <= N; n++) {
    for (let m = 0; m < n; m++) {
      const k = Math.sqrt(n * n - m * m), k2 = n - 2 >= m ? Math.sqrt((n - 1) * (n - 1) - m * m) : 0;
      const p2 = n - 2 >= m ? P[n - 2][m] : 0, d2 = n - 2 >= m ? dP[n - 2][m] : 0;
      P[n][m] = ((2 * n - 1) * c * P[n - 1][m] - k2 * p2) / k;
      dP[n][m] = ((2 * n - 1) * (c * dP[n - 1][m] - s * P[n - 1][m]) - k2 * d2) / k;
    }
    const f = n === 1 ? 1 : Math.sqrt((2 * n - 1) / (2 * n));
    P[n][n] = f * s * P[n - 1][n - 1];
    dP[n][n] = f * (s * dP[n - 1][n - 1] + c * P[n - 1][n - 1]);
  }
  // the field: north (X), east (Y) and down (Z) in the geocentric frame
  let X = 0, Y = 0, Z = 0;
  const lam = lon * rad;
  for (const [n, m, g0, h0, gd, hd] of COF) {
    const g = g0 + dt * gd, h = h0 + dt * hd, ar = (A / r) ** (n + 2);
    const cm = Math.cos(m * lam), sm = Math.sin(m * lam);
    X += ar * (g * cm + h * sm) * dP[n][m];
    Y += ar * m * (g * sm - h * cm) * P[n][m];
    Z -= ar * (n + 1) * (g * cm + h * sm) * P[n][m];
  }
  Y = s > 1e-10 ? Y / s : 0;
  // to the geodetic frame (Y stays)
  const psi = phiC - phi;
  const Xg = X * Math.cos(psi) - Z * Math.sin(psi);
  return Math.atan2(Y, Xg) / rad;
}
