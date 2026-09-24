package video

import (
	"context"
	"math"
	"time"
)

// headingLookback is how far back along the track the bearing is measured. A
// dashcam clip's coordinates are noisy metre-to-metre, so a short baseline
// reads as a compass needle twitching; ten seconds of driving is long enough
// for the noise to wash out and short enough that a bend still shows up.
const headingLookback = 10 * time.Second

// stoppedMetres is the distance below which the two samples are treated as the
// same place. GPS scatter alone moves a parked van several metres, so a bearing
// computed under this is noise with a number attached.
const stoppedMetres = 15

// earthRadiusM is the mean radius, for the haversine distance below.
const earthRadiusM = 6371000

// compassPoints are the eight headings chat hears, in the order the bearing
// walks through them from north.
var compassPoints = [8]string{
	"north", "northeast", "east", "southeast",
	"south", "southwest", "west", "northwest",
}

// Compass names the eight-point direction a bearing falls in. Chat gets a word
// rather than a number of degrees: "heading northwest" is what a viewer can
// picture, and eight points is already finer than a dashcam track supports.
func Compass(bearing float64) string {
	// +22.5 puts the boundary between two points halfway between them rather
	// than on one of them, so due north stays "north" for 22.5° either side.
	idx := int(math.Floor(math.Mod(bearing+22.5+360, 360) / 45))
	return compassPoints[idx%8]
}

// Velocity is how the van was moving at one moment: which way, and how fast.
type Velocity struct {
	// Bearing is the direction of travel in degrees clockwise from north.
	// Meaningless when Moving is false.
	Bearing float64
	// SpeedMPS is the ground speed in metres per second, averaged over the
	// headingLookback baseline. 0 when Moving is false.
	SpeedMPS float64
	// Moving is false when the two samples sit closer together than
	// stoppedMetres — a parked van, or GPS scatter that would otherwise read
	// as a bearing.
	Moving bool
}

// VelocityAt reports which way and how fast the van was travelling at `at`
// into vid.
//
// It reads the per-moment track twice — once at `at` and once a
// headingLookback earlier — and takes the initial great-circle bearing from
// the older sample to the newer, and the distance between them over the
// baseline as the speed. Moving is false when the two samples are closer
// together than stoppedMetres, which is the caller's signal to say the van is
// stopped rather than to name a direction. ok is false when the clip has no
// track covering both moments, exactly as CoordAt reports it.
//
// ponytail: the speed is a chord over ten seconds, which under-reads a tight
// bend by a few percent; a track integral if that ever matters.
func VelocityAt(ctx context.Context, vid Video, at time.Duration) (v Velocity, ok bool) {
	// Near the top of a clip there is nothing behind the playhead to measure
	// against, so the baseline runs forward from the start instead. The
	// direction a few seconds either side of the opening is the same road.
	from, to := at-headingLookback, at
	if from < 0 {
		from, to = 0, headingLookback
	}

	start, ok := CoordAt(ctx, vid, from)
	if !ok {
		return Velocity{}, false
	}
	end, ok := CoordAt(ctx, vid, to)
	if !ok {
		return Velocity{}, false
	}

	dist := distanceM(start, end)
	if dist < stoppedMetres {
		return Velocity{}, true
	}
	return Velocity{
		Bearing:  bearingDeg(start, end),
		SpeedMPS: dist / (to - from).Seconds(),
		Moving:   true,
	}, true
}

// MPH and KPH render a Velocity in the units chat reads.
func (v Velocity) MPH() float64 { return v.SpeedMPS * 2.236936 }
func (v Velocity) KPH() float64 { return v.SpeedMPS * 3.6 }

// bearingDeg is the initial great-circle bearing from a to b, in degrees
// clockwise from north.
func bearingDeg(a, b Moment) float64 {
	lat1, lat2 := rad(a.Lat), rad(b.Lat)
	dLon := rad(b.Lng - a.Lng)

	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	return math.Mod(deg(math.Atan2(y, x))+360, 360)
}

// distanceM is the haversine distance between two moments, in metres.
func distanceM(a, b Moment) float64 {
	lat1, lat2 := rad(a.Lat), rad(b.Lat)
	dLat, dLon := rad(b.Lat-a.Lat), rad(b.Lng-a.Lng)

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(h))
}

func rad(d float64) float64 { return d * math.Pi / 180 }
func deg(r float64) float64 { return r * 180 / math.Pi }
