// Copyright 2013 Andreas Koch. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package thumbnail

import (
	"fmt"
	"github.com/andreaskoch/allmark2/common/config"
	"github.com/andreaskoch/allmark2/common/logger"
	"github.com/andreaskoch/allmark2/common/route"
	"github.com/andreaskoch/allmark2/common/shutdown"
	"github.com/andreaskoch/allmark2/common/util/fsutil"
	"github.com/andreaskoch/allmark2/dataaccess"
	"github.com/andreaskoch/allmark2/services/imageconversion"
	"io"
	"path/filepath"
	"time"
)

func NewConversionService(logger logger.Logger, config config.Config, repository dataaccess.Repository) *ConversionService {

	// assemble the index file path
	indexFilePath := filepath.Join(config.MetaDataFolder(), "thumbnail.index")
	index, err := loadIndex(indexFilePath)
	if err != nil {
		logger.Debug("No thumbnail index loaded (%s). Creating a new one.", err.Error())
	}

	// prepare the target folder
	targetFolder := filepath.Join(config.MetaDataFolder(), "thumbnails")
	logger.Debug("Creating a thumbnail folder at %q.", targetFolder)
	if !fsutil.CreateDirectory(targetFolder) {
		logger.Warn("Could not create the thumbnail folder %q", targetFolder)
		return nil
	}

	// create a new conversion service
	conversionService := &ConversionService{
		logger:     logger,
		config:     config,
		repository: repository,

		isRunning: true,

		// thumbnail index
		indexFilePath: indexFilePath,
		index:         index,

		thumbnailFolder: targetFolder,
	}

	// start the conversion
	go conversionService.startConversion()

	// stop the conversion on shutdown
	shutdown.Register(func() error {
		logger.Info("Stopping the conversion process")
		conversionService.isRunning = false
		return nil
	})

	// save the index on shutdown
	shutdown.Register(func() error {
		logger.Info("Saving the index")
		return saveIndex(index, indexFilePath)
	})

	return conversionService
}

type ConversionService struct {
	logger     logger.Logger
	config     config.Config
	repository dataaccess.Repository

	isRunning bool

	indexFilePath string
	index         Index

	thumbnailFolder string
}

func (conversion *ConversionService) startConversion() {

	conversion.createThumbnails()

	updateChannel := make(chan bool, 1)
	conversion.repository.AfterReindex(updateChannel)

	// refresh control
	go func() {
		for conversion.isRunning {
			select {
			case <-updateChannel:
				conversion.logger.Debug("Refreshing thumbnails")
				conversion.createThumbnails()
			}
		}
	}()

}

func (conversion *ConversionService) createThumbnails() {
	for _, item := range conversion.repository.Items() {

		for _, file := range item.Files() {

			// create the thumbnail
			conversion.createThumbnail(file, 200, 0)
			conversion.createThumbnail(file, 400, 0)
			conversion.createThumbnail(file, 800, 0)

			// wait before processing the next image
			time.Sleep(5 * time.Second)
		}
	}
}

func (conversion *ConversionService) createThumbnail(file *dataaccess.File, maxWidth, maxHeight uint) {

	// get the mime type
	mimeType, err := file.MimeType()
	if err != nil {
		conversion.logger.Warn("Unable to detect mime type for file. Error: %s", err.Error())
		return
	}

	// check the mime type
	if !imageconversion.MimeTypeIsSupported(mimeType) {
		conversion.logger.Debug("The mime-type %q is currently not supported.", mimeType)
		return
	}

	// determine the file name
	fileExtension := imageconversion.GetFileExtensionFromMimeType(mimeType)
	filename := fmt.Sprintf("%s-%v-%v.%s", file.Id(), maxWidth, maxHeight, fileExtension)

	// assemble the full file route
	fullFileRoute, err := route.Combine(file.Parent(), file.Route())
	if err != nil {
		conversion.logger.Warn("Unable to combine routes %q and %q.", file.Parent(), file.Route())
		return
	}

	thumb := newThumb(fullFileRoute, filename, maxWidth, maxHeight)

	// check the index
	if conversion.isInIndex(thumb) {
		conversion.logger.Debug("Thumb %q already available in the index", thumb.String())
		return
	}

	// determine the file path
	filePath := filepath.Join(conversion.thumbnailFolder, filename)

	// open the target file
	target, err := fsutil.OpenFile(filePath)
	if err != nil {
		conversion.logger.Warn("Unable to detect mime type for file. Error: %s", err.Error())
		return
	}

	defer target.Close()

	// convert the image
	conversionError := file.Data(func(content io.ReadSeeker) error {
		return imageconversion.Resize(content, mimeType, maxWidth, maxHeight, target)
	})

	// handle errors
	if conversionError != nil {
		conversion.logger.Warn("Unable to create thumbnail for file %q. Error: %s", file, err.Error())
		return
	}

	// add to index
	conversion.addToIndex(thumb)
	conversion.logger.Debug("Adding Thumb %q to index", thumb.String())
}

func (conversion *ConversionService) isInIndex(thumb Thumb) bool {
	thumbs, entryExists := conversion.index[thumb.Route]
	if !entryExists {
		return false
	}

	_, thumbExists := thumbs[thumb.Dimensions.String()]
	return thumbExists
}

func (conversion *ConversionService) addToIndex(thumb Thumb) {
	thumbs, entryExists := conversion.index[thumb.Route]
	if !entryExists {
		thumbs = make(Thumbs)
	}

	thumbs[thumb.Dimensions.String()] = thumb
	conversion.index[thumb.Route] = thumbs
}
