package admin

// Thumbnail misses must not fan out into dozens of full image decodes.
var thumbnailRenderSlot = make(chan struct{}, 1)

// Thumbnail files are small; never cache source images here.
